package facility

import (
	"testing"
	"time"
)

func TestCoolingLagApproachesSteadyPUE(t *testing.T) {
	model, err := New(1.2, 60)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	first := model.Step(100, time.Second)
	if first.FacilityMW <= 100 || first.FacilityMW >= 120 {
		t.Fatalf("expected lagged facility between IT and steady state, got %+v", first)
	}
	var last Step
	for i := 0; i < 600; i++ {
		last = model.Step(100, time.Second)
	}
	if last.FacilityMW < 119.9 || last.FacilityMW > 120.1 || last.PUE < 1.199 || last.PUE > 1.201 {
		t.Fatalf("steady step=%+v", last)
	}
}

func TestZeroLagUsesSteadyStateImmediately(t *testing.T) {
	model, err := New(1.15, 0)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	step := model.Step(80, time.Second)
	if step.FacilityMW < 91.999 || step.FacilityMW > 92.001 || step.CoolingMW < 11.999 || step.CoolingMW > 12.001 {
		t.Fatalf("unexpected zero-lag step %+v", step)
	}
}
