package metrics

import (
	"testing"
	"time"

	"grid-dynamics-modelling/internal/pmu"
	"grid-dynamics-modelling/internal/scenario"
)

func TestComputeReportsRampReductionAndViolations(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	baseline := []Point{{Timestamp: start, NetGridMW: 0, FrequencyHz: 60, VoltagePU: 1}, {Timestamp: start.Add(time.Minute), NetGridMW: 20, FrequencyHz: 60, VoltagePU: 1}}
	controlled := []Point{{Timestamp: start, NetGridMW: 0, BESSSOC: 0.6, FrequencyHz: 60, VoltagePU: 1}, {Timestamp: start.Add(time.Minute), NetGridMW: 4, BESSPowerMW: 1, BESSSOC: 0.59, FrequencyHz: 60, VoltagePU: 1}}
	cfg := scenario.Config{Controller: scenario.ControllerConfig{RampLimitMWPerMin: 5, RecoveryWindow: "30m", EventFrequencyLowHz: 59.95, EventVoltageLowPU: 0.95}}
	summary := Compute(baseline, controlled, cfg, pmu.EventSummary{})
	if summary.PeakBaselineRampMWPerMin != 20 || summary.PeakControlledRampMWPerMin != 4 || summary.PeakRampRateReductionMWPerMin != 16 {
		t.Fatalf("unexpected ramp metrics %+v", summary)
	}
	if summary.RampRateViolationCount != 0 || !summary.Feasible {
		t.Fatalf("expected feasible controlled case %+v", summary)
	}
}

func TestComputeReportsInfeasibleRampAndRideThrough(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	points := []Point{{Timestamp: start, NetGridMW: 0, FrequencyHz: 60, VoltagePU: 1}, {Timestamp: start.Add(time.Minute), NetGridMW: 20, FrequencyHz: 59.9, VoltagePU: 0.94}}
	cfg := scenario.Config{Controller: scenario.ControllerConfig{RampLimitMWPerMin: 5, RecoveryWindow: "30m", EventFrequencyLowHz: 59.95, EventVoltageLowPU: 0.95}}
	summary := Compute(points, points, cfg, pmu.EventSummary{})
	if summary.RampRateViolationCount != 1 || summary.WorstRampRateViolationMWPerMin != 15 || summary.RideThroughViolationCount != 1 || summary.Feasible {
		t.Fatalf("expected infeasible summary %+v", summary)
	}
}
