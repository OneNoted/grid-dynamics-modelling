package control

import (
	"testing"
	"time"

	"grid-dynamics-modelling/internal/workload"
)

func TestPolicyDefersDuringEventAndCapsRamp(t *testing.T) {
	p := New(5, 30*time.Minute)
	d := workload.Demand{ITMW: 100, UrgentMW: 75, DeferrableMW: 25}
	decision := p.Decide(d, 0, true, time.Minute, 80, 100)
	if decision.DeliveredITMW != 75 || decision.DeferredMW != 25 {
		t.Fatalf("unexpected deferral decision %+v", decision)
	}
	if decision.BESSRequestMW < 14.99 || decision.BESSRequestMW > 15.01 {
		t.Fatalf("expected ramp-capping BESS request 15 MW, got %+v", decision)
	}
}

func TestPolicyRecoversDeferredWorkWithinWindow(t *testing.T) {
	p := New(5, 30*time.Minute)
	d := workload.Demand{ITMW: 80, UrgentMW: 60, DeferrableMW: 20}
	decision := p.Decide(d, 15, false, time.Minute, 80, 80)
	if decision.RecoveredMW < 29.9 || decision.RecoveredMW > 30.1 {
		t.Fatalf("expected 30 MW recovery over 30m, got %+v", decision)
	}
	if decision.DeliveredITMW < 109.9 || decision.DeliveredITMW > 110.1 {
		t.Fatalf("unexpected delivered load %+v", decision)
	}
}

func TestPolicyRequestsChargingToCapDownwardRamp(t *testing.T) {
	p := New(5, 30*time.Minute)
	request := p.BESSRequest(50, 40, time.Minute)
	if request > -4.99 || request < -5.01 {
		t.Fatalf("expected -5 MW charge request, got %f", request)
	}
}
