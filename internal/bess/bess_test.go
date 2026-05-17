package bess

import (
	"testing"
	"time"
)

func TestDispatchEnforcesPowerAndSoCLimits(t *testing.T) {
	bat, err := New(Config{PowerMW: 10, EnergyMWh: 1, InitialSOC: 0.6, MinSOC: 0.2, MaxSOC: 1})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	out := bat.Dispatch(20, time.Hour)
	if out.ActualMW < 0.399 || out.ActualMW > 0.401 {
		t.Fatalf("expected energy-limited discharge to 0.4 MW, got %+v", out)
	}
	if bat.SOC < 0.199 || bat.SOC > 0.201 {
		t.Fatalf("expected min SoC, got %f", bat.SOC)
	}
	out = bat.Dispatch(5, time.Minute)
	if out.ActualMW != 0 || out.UnmetMW != 5 {
		t.Fatalf("expected no discharge at min SoC, got %+v", out)
	}
}

func TestDispatchSupportsChargingToMaxSoC(t *testing.T) {
	bat, err := New(Config{PowerMW: 10, EnergyMWh: 1, InitialSOC: 0.95, MinSOC: 0.2, MaxSOC: 1})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	out := bat.Dispatch(-10, time.Hour)
	if out.ActualMW < -0.051 || out.ActualMW > -0.049 {
		t.Fatalf("expected room-limited charge, got %+v", out)
	}
	if bat.SOC < 0.999 || bat.SOC > 1.001 {
		t.Fatalf("expected max SoC, got %f", bat.SOC)
	}
}
