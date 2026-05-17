package runs

import (
	"bytes"
	"strings"
	"testing"

	"grid-dynamics-modelling/internal/pmu"
	"grid-dynamics-modelling/internal/scenario"
)

func TestManifestWriteJSONIncludesStableSchemaAndSources(t *testing.T) {
	cfg := scenario.Config{
		Name:       "demo",
		Simulation: scenario.SimulationConfig{Start: "2026-01-01T00:00:00Z", Duration: "60m", Step: "1s"},
		PMU:        scenario.PMUConfig{File: "data/samples/pmu_event_tiny.csv", FrequencyColumn: "frequency_hz", VoltageColumn: "voltage_pu"},
		Workload:   scenario.WorkloadConfig{Source: "data/samples/genai_power_tiny.csv"},
		Controller: scenario.ControllerConfig{RampLimitMWPerMin: 5, RecoveryWindow: "30m", EventFrequencyLowHz: 59.95},
	}
	manifest := NewManifest(cfg, pmu.Series{})
	var buf bytes.Buffer
	if err := manifest.WriteJSON(&buf); err != nil {
		t.Fatalf("WriteJSON returned error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{ManifestSchemaVersion, "demo", "data/samples/pmu_event_tiny.csv", "Synthetic GenAI workload tiny fixture"} {
		if !strings.Contains(out, want) {
			t.Fatalf("manifest JSON missing %q:\n%s", want, out)
		}
	}
}
