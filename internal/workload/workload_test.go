package workload

import (
	"strings"
	"testing"
	"time"
)

func TestTraceLoadsAndScalesDemand(t *testing.T) {
	trace, err := ParseCSV(strings.NewReader(`timestamp,workload_id,workload_kind,source,gpu_count,it_power_kw,deferrable_fraction,urgency_class,metadata
2026-01-01T00:00:00Z,w,training,synthetic,8,50,0.25,normal,{}
2026-01-01T00:00:10Z,w,training,synthetic,8,100,0.25,normal,{}
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	model, err := NewModel(trace, 80, 5*time.Minute, 10*time.Minute, 0.25)
	if err != nil {
		t.Fatalf("model: %v", err)
	}
	demand := model.DemandAt(start.Add(10*time.Minute), start)
	if demand.RampFraction < 0.49 || demand.RampFraction > 0.51 {
		t.Fatalf("ramp fraction=%f", demand.RampFraction)
	}
	if demand.ITMW < 39.9 || demand.ITMW > 40.1 {
		t.Fatalf("ITMW=%f", demand.ITMW)
	}
	if demand.DeferrableMW < 9.9 || demand.DeferrableMW > 10.1 || demand.UrgentMW < 29.9 || demand.UrgentMW > 30.1 {
		t.Fatalf("split urgent=%f deferrable=%f", demand.UrgentMW, demand.DeferrableMW)
	}
}

func TestQueueDefersAndRecoversWork(t *testing.T) {
	var q Queue
	step := q.Step(12, true, 0, 5*time.Minute)
	if step.DeferredNowMWh < 0.99 || step.DeferredNowMWh > 1.01 || q.DeferredMWh < 0.99 || q.DeferredMWh > 1.01 {
		t.Fatalf("deferred step=%+v queue=%f", step, q.DeferredMWh)
	}
	step = q.Step(12, false, 6, 10*time.Minute)
	if step.RecoveredMWh < 0.99 || step.RecoveredMWh > 1.01 || q.DeferredMWh > 1e-9 {
		t.Fatalf("recovered step=%+v queue=%f", step, q.DeferredMWh)
	}
}

func TestQueueReportsInfeasibleRecoveryWithoutCapacity(t *testing.T) {
	q := Queue{DeferredMWh: 1}
	step := q.Step(0, false, 0, time.Minute)
	if !step.InfeasibleRecovery || step.RemainingMWh != 1 {
		t.Fatalf("expected infeasible recovery, got %+v", step)
	}
}

func TestParseRejectsBadColumns(t *testing.T) {
	_, err := ParseCSV(strings.NewReader("timestamp,it_power_kw\n2026-01-01T00:00:00Z,1\n"))
	if err == nil || !strings.Contains(err.Error(), "missing required workload column") {
		t.Fatalf("expected missing column error, got %v", err)
	}
}
