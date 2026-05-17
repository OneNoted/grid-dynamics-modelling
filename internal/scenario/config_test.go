package scenario

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validConfig() Config {
	return Config{
		Name:       "demo",
		Simulation: SimulationConfig{Start: "2026-01-01T00:00:00Z", Duration: "10m", Step: "1s"},
		PMU:        PMUConfig{File: "data/samples/pmu_event_tiny.csv", ResampleStep: "1s"},
		Workload:   WorkloadConfig{Source: "data/samples/genai_power_tiny.csv", ScaleMW: 80, RampStart: "5m", RampDuration: "10m", DeferrableFraction: 0.25},
		Facility:   FacilityConfig{CoolingLagSeconds: 180, PUEBase: 1.15},
		BESS:       BESSConfig{PowerMW: 20, EnergyMWh: 10, InitialSOC: 0.60, MinSOC: 0.20},
		Controller: ControllerConfig{RampLimitMWPerMin: 5, RecoveryWindow: "30m", EventFrequencyLowHz: 59.95, EventVoltageLowPU: 0.95},
	}
}

func TestValidateAcceptsValidConfig(t *testing.T) {
	if err := validConfig().Validate(); err != nil {
		t.Fatalf("expected valid config: %v", err)
	}
}

func TestLoadJSONResolvesDataPathsRelativeToScenarioFile(t *testing.T) {
	dir := t.TempDir()
	scenarioDir := filepath.Join(dir, "nested")
	if err := os.MkdirAll(scenarioDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := validConfig()
	cfg.PMU.File = "pmu.csv"
	cfg.Workload.Source = "workload.csv"
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(scenarioDir, "scenario.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadJSON(path)
	if err != nil {
		t.Fatalf("LoadJSON: %v", err)
	}
	if loaded.PMU.File != filepath.Join(scenarioDir, "pmu.csv") {
		t.Fatalf("pmu path=%q", loaded.PMU.File)
	}
	if loaded.Workload.Source != filepath.Join(scenarioDir, "workload.csv") {
		t.Fatalf("workload path=%q", loaded.Workload.Source)
	}
}

func TestValidateReturnsActionableErrors(t *testing.T) {
	cfg := validConfig()
	cfg.Name = ""
	cfg.Workload.DeferrableFraction = 1.5
	cfg.BESS.InitialSOC = 0.1
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error")
	}
	msg := err.Error()
	for _, want := range []string{"name is required", "workload.deferrable_fraction", "bess.initial_soc"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("expected %q in error %q", want, msg)
		}
	}
}
