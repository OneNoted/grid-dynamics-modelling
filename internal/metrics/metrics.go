package metrics

import (
	"math"
	"time"

	"grid-dynamics-modelling/internal/pmu"
	"grid-dynamics-modelling/internal/scenario"
)

type Point struct {
	Timestamp        time.Time
	NetGridMW        float64
	DeferredQueueMWh float64
	BESSPowerMW      float64
	BESSSOC          float64
	FrequencyHz      float64
	VoltagePU        float64
	EventActive      bool
}

type Summary struct {
	RampLimitMWPerMin              float64 `json:"ramp_limit_mw_per_min"`
	PeakBaselineRampMWPerMin       float64 `json:"peak_baseline_ramp_mw_per_min"`
	PeakControlledRampMWPerMin     float64 `json:"peak_controlled_ramp_mw_per_min"`
	PeakRampRateReductionMWPerMin  float64 `json:"peak_ramp_rate_reduction_mw_per_min"`
	MaxGridPowerDeviationMW        float64 `json:"max_grid_power_deviation_mw"`
	WorkloadRecoveryTimeSeconds    float64 `json:"workload_recovery_time_seconds"`
	DeferredWorkMWh                float64 `json:"deferred_work_mwh"`
	MaxDeferredQueueMWh            float64 `json:"max_deferred_queue_mwh"`
	BESSEnergyDischargedMWh        float64 `json:"bess_energy_discharged_mwh"`
	BESSMinSOC                     float64 `json:"bess_min_soc"`
	BESSMaxSOC                     float64 `json:"bess_max_soc"`
	EventDetectionLatencySeconds   float64 `json:"event_detection_latency_seconds"`
	RampRateViolationCount         int     `json:"ramp_rate_violation_count"`
	WorstRampRateViolationMWPerMin float64 `json:"worst_ramp_rate_violation_mw_per_min"`
	RideThroughViolationCount      int     `json:"ride_through_violation_count"`
	Feasible                       bool    `json:"feasible"`
}

func Compute(baseline, controlled []Point, cfg scenario.Config, events pmu.EventSummary) Summary {
	limit := cfg.Controller.RampLimitMWPerMin
	basePeak := peakAbsRamp(baseline)
	controlledPeak := peakAbsRamp(controlled)
	s := Summary{RampLimitMWPerMin: limit, PeakBaselineRampMWPerMin: basePeak, PeakControlledRampMWPerMin: controlledPeak, PeakRampRateReductionMWPerMin: basePeak - controlledPeak, BESSMinSOC: math.Inf(1)}
	for i, p := range controlled {
		if i < len(baseline) {
			s.MaxGridPowerDeviationMW = math.Max(s.MaxGridPowerDeviationMW, math.Abs(p.NetGridMW-baseline[i].NetGridMW))
		}
		s.MaxDeferredQueueMWh = math.Max(s.MaxDeferredQueueMWh, p.DeferredQueueMWh)
		s.DeferredWorkMWh = math.Max(s.DeferredWorkMWh, p.DeferredQueueMWh)
		if p.BESSPowerMW > 0 && i > 0 {
			s.BESSEnergyDischargedMWh += p.BESSPowerMW * p.Timestamp.Sub(controlled[i-1].Timestamp).Hours()
		}
		if p.BESSSOC > 0 {
			s.BESSMinSOC = math.Min(s.BESSMinSOC, p.BESSSOC)
			s.BESSMaxSOC = math.Max(s.BESSMaxSOC, p.BESSSOC)
		}
		if violatesRideThrough(p, cfg) {
			s.RideThroughViolationCount++
		}
	}
	if math.IsInf(s.BESSMinSOC, 1) {
		s.BESSMinSOC = 0
	}
	s.RampRateViolationCount, s.WorstRampRateViolationMWPerMin = rampViolations(controlled, limit)
	s.WorkloadRecoveryTimeSeconds = recoveryTime(controlled)
	if len(events.Events) > 0 {
		s.EventDetectionLatencySeconds = events.Events[0].DetectionTime.Sub(events.Events[0].Start).Seconds()
	}
	s.Feasible = s.RampRateViolationCount == 0 && s.WorkloadRecoveryTimeSeconds <= mustDuration(cfg.Controller.RecoveryWindow).Seconds()
	return s
}

func peakAbsRamp(points []Point) float64 {
	peak := 0.0
	for i := 1; i < len(points); i++ {
		dt := points[i].Timestamp.Sub(points[i-1].Timestamp).Minutes()
		if dt <= 0 {
			continue
		}
		peak = math.Max(peak, math.Abs((points[i].NetGridMW-points[i-1].NetGridMW)/dt))
	}
	return peak
}

func rampViolations(points []Point, limit float64) (int, float64) {
	count := 0
	worst := 0.0
	for i := 1; i < len(points); i++ {
		dt := points[i].Timestamp.Sub(points[i-1].Timestamp).Minutes()
		if dt <= 0 {
			continue
		}
		ramp := math.Abs((points[i].NetGridMW - points[i-1].NetGridMW) / dt)
		if ramp > limit+1e-9 {
			count++
			worst = math.Max(worst, ramp-limit)
		}
	}
	return count, worst
}

func recoveryTime(points []Point) float64 {
	var start *time.Time
	var end *time.Time
	for _, p := range points {
		if p.DeferredQueueMWh > 1e-3 && start == nil {
			t := p.Timestamp
			start = &t
		}
		if start != nil && end == nil && p.DeferredQueueMWh <= 1e-3 {
			t := p.Timestamp
			end = &t
		}
	}
	if start == nil {
		return 0
	}
	if end == nil {
		last := points[len(points)-1].Timestamp
		return last.Sub(*start).Seconds()
	}
	return end.Sub(*start).Seconds()
}

func violatesRideThrough(p Point, cfg scenario.Config) bool {
	c := cfg.Controller
	return (c.EventFrequencyLowHz > 0 && p.FrequencyHz < c.EventFrequencyLowHz) || (c.EventFrequencyHighHz > 0 && p.FrequencyHz > c.EventFrequencyHighHz) || (c.EventVoltageLowPU > 0 && p.VoltagePU < c.EventVoltageLowPU) || (c.EventVoltageHighPU > 0 && p.VoltagePU > c.EventVoltageHighPU)
}

func mustDuration(raw string) time.Duration {
	d, _ := time.ParseDuration(raw)
	return d
}
