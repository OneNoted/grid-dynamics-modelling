package sim

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"testing"

	"grid-dynamics-modelling/internal/scenario"
)

func TestRunBaselineEmitsDeterministicSeriesAndArtifacts(t *testing.T) {
	cfg, err := scenario.LoadJSON("../../scenarios/demo.json")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.Simulation.Duration = "10s"
	cfg.PMU.File = "../../" + cfg.PMU.File
	cfg.Workload.Source = "../../" + cfg.Workload.Source
	result, err := Run(cfg)
	if err != nil {
		t.Fatalf("run baseline: %v", err)
	}
	if len(result.Baseline) != 11 || len(result.Controlled) != 11 {
		t.Fatalf("expected 11 samples, got %d", len(result.Baseline))
	}
	if result.Baseline[0].Mode != ModeBaseline || result.Baseline[0].FrequencyHz == 0 || result.Baseline[0].NetGridMW < 0 {
		t.Fatalf("unexpected first point: %+v", result.Baseline[0])
	}
	if result.Events.Count != 1 {
		t.Fatalf("expected one detected event, got %+v", result.Events)
	}
	dir := t.TempDir()
	if err := WriteBaselineArtifacts(dir, result); err != nil {
		t.Fatalf("write artifacts: %v", err)
	}
	for _, name := range []string{"manifest.json", "events.json", "baseline_timeseries.csv", "controlled_timeseries.csv", "metrics.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("missing artifact %s: %v", name, err)
		}
	}
	f, err := os.Open(filepath.Join(dir, "baseline_timeseries.csv"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	records, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatalf("read timeseries: %v", err)
	}
	if len(records) != len(result.Baseline)+1 {
		t.Fatalf("CSV rows=%d baseline=%d", len(records), len(result.Baseline))
	}
}

func TestControlledRunReportsFeasibleAndInfeasibleFixtures(t *testing.T) {
	cfg, err := scenario.LoadJSON("../../scenarios/demo.json")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.Simulation.Duration = "20m"
	cfg.PMU.File = "../../" + cfg.PMU.File
	cfg.Workload.Source = "../../" + cfg.Workload.Source
	cfg.Workload.ScaleMW = 80
	cfg.BESS.PowerMW = 100
	cfg.BESS.EnergyMWh = 50
	cfg.BESS.InitialSOC = 0.9
	feasible, err := Run(cfg)
	if err != nil {
		t.Fatalf("run feasible: %v", err)
	}
	if !feasible.Metrics.Feasible || feasible.Metrics.RampRateViolationCount != 0 {
		t.Fatalf("expected feasible fixture, got %+v", feasible.Metrics)
	}

	cfg.BESS.PowerMW = 0.1
	cfg.BESS.EnergyMWh = 0.1
	cfg.BESS.InitialSOC = 0.5
	infeasible, err := Run(cfg)
	if err != nil {
		t.Fatalf("run infeasible: %v", err)
	}
	if infeasible.Metrics.Feasible || infeasible.Metrics.RampRateViolationCount == 0 {
		t.Fatalf("expected infeasible fixture, got %+v", infeasible.Metrics)
	}
}
