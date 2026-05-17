package sim

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"grid-dynamics-modelling/internal/bess"
	"grid-dynamics-modelling/internal/control"
	"grid-dynamics-modelling/internal/facility"
	"grid-dynamics-modelling/internal/metrics"
	"grid-dynamics-modelling/internal/pmu"
	"grid-dynamics-modelling/internal/runs"
	"grid-dynamics-modelling/internal/scenario"
	"grid-dynamics-modelling/internal/workload"
)

type Mode string

const (
	ModeBaseline Mode = "baseline"
)

type Point struct {
	Timestamp        time.Time `json:"timestamp"`
	Mode             Mode      `json:"mode"`
	FrequencyHz      float64   `json:"frequency_hz"`
	VoltagePU        float64   `json:"voltage_pu"`
	ITMW             float64   `json:"it_mw"`
	UrgentMW         float64   `json:"urgent_mw"`
	DeferrableMW     float64   `json:"deferrable_mw"`
	CoolingMW        float64   `json:"cooling_mw"`
	FacilityMW       float64   `json:"facility_mw"`
	BESSPowerMW      float64   `json:"bess_power_mw"`
	BESSSOC          float64   `json:"bess_soc"`
	NetGridMW        float64   `json:"net_grid_mw"`
	DeferredQueueMWh float64   `json:"deferred_queue_mwh"`
	EventActive      bool      `json:"event_active"`
}

type Result struct {
	Config     scenario.Config  `json:"config"`
	Manifest   runs.Manifest    `json:"manifest"`
	Events     pmu.EventSummary `json:"events"`
	Baseline   []Point          `json:"baseline"`
	Controlled []Point          `json:"controlled,omitempty"`
	Metrics    metrics.Summary  `json:"metrics,omitempty"`
	PMUSummary pmu.Summary      `json:"pmu_summary"`
}

func Run(cfg scenario.Config) (Result, error) {
	result, err := RunBaseline(cfg)
	if err != nil {
		return Result{}, err
	}
	controlled, err := runControlled(cfg, result.Events)
	if err != nil {
		return Result{}, err
	}
	result.Controlled = controlled
	result.Metrics = metrics.Compute(toMetricPoints(result.Baseline), toMetricPoints(controlled), cfg, result.Events)
	return result, nil
}

func runControlled(cfg scenario.Config, events pmu.EventSummary) ([]Point, error) {
	start, duration, step, err := parseSimulation(cfg.Simulation)
	if err != nil {
		return nil, err
	}
	pmuSeries, err := loadPMU(cfg)
	if err != nil {
		return nil, err
	}
	trace, err := workload.LoadFile(cfg.Workload.Source)
	if err != nil {
		return nil, err
	}
	rampStart, err := time.ParseDuration(cfg.Workload.RampStart)
	if err != nil {
		return nil, fmt.Errorf("parse workload.ramp_start: %w", err)
	}
	rampDuration, err := time.ParseDuration(cfg.Workload.RampDuration)
	if err != nil {
		return nil, fmt.Errorf("parse workload.ramp_duration: %w", err)
	}
	recoveryWindow, err := time.ParseDuration(cfg.Controller.RecoveryWindow)
	if err != nil {
		return nil, fmt.Errorf("parse controller.recovery_window: %w", err)
	}
	workloadModel, err := workload.NewModel(trace, cfg.Workload.ScaleMW, rampStart, rampDuration, cfg.Workload.DeferrableFraction)
	if err != nil {
		return nil, err
	}
	facilityModel, err := facility.New(cfg.Facility.PUEBase, cfg.Facility.CoolingLagSeconds)
	if err != nil {
		return nil, err
	}
	battery, err := bess.New(bess.Config{PowerMW: cfg.BESS.PowerMW, EnergyMWh: cfg.BESS.EnergyMWh, InitialSOC: cfg.BESS.InitialSOC, MinSOC: cfg.BESS.MinSOC, MaxSOC: cfg.BESS.MaxSOC})
	if err != nil {
		return nil, err
	}
	policy := control.New(cfg.Controller.RampLimitMWPerMin, recoveryWindow)
	var queue workload.Queue
	controlled := make([]Point, 0, int(duration/step)+1)
	previousNet := 0.0
	for ts := start; !ts.After(start.Add(duration)); ts = ts.Add(step) {
		demand := workloadModel.DemandAt(ts, start)
		eventActive := isEventActive(events, ts)
		decision := policy.Decide(demand, queue.DeferredMWh, eventActive, step, previousNet, 0)
		if eventActive {
			queue.Step(demand.DeferrableMW, true, 0, step)
		} else {
			queue.Step(0, false, decision.RecoveredMW, step)
		}
		fac := facilityModel.Step(decision.DeliveredITMW, step)
		dispatch := battery.Dispatch(policy.BESSRequest(previousNet, fac.FacilityMW, step), step)
		pmuSample := sampleAt(pmuSeries.Samples, ts)
		net := fac.FacilityMW - dispatch.ActualMW
		controlled = append(controlled, Point{
			Timestamp: ts.UTC(), Mode: "controlled", FrequencyHz: pmuSample.FrequencyHz, VoltagePU: pmuSample.VoltagePU,
			ITMW: decision.DeliveredITMW, UrgentMW: demand.UrgentMW, DeferrableMW: demand.DeferrableMW, CoolingMW: fac.CoolingMW,
			FacilityMW: fac.FacilityMW, BESSPowerMW: dispatch.ActualMW, BESSSOC: dispatch.SOC, NetGridMW: net,
			DeferredQueueMWh: queue.DeferredMWh, EventActive: eventActive,
		})
		previousNet = net
	}
	return controlled, nil
}

func RunBaseline(cfg scenario.Config) (Result, error) {
	start, duration, step, err := parseSimulation(cfg.Simulation)
	if err != nil {
		return Result{}, err
	}
	pmuSeries, err := loadPMU(cfg)
	if err != nil {
		return Result{}, err
	}
	trace, err := workload.LoadFile(cfg.Workload.Source)
	if err != nil {
		return Result{}, err
	}
	rampStart, err := time.ParseDuration(cfg.Workload.RampStart)
	if err != nil {
		return Result{}, fmt.Errorf("parse workload.ramp_start: %w", err)
	}
	rampDuration, err := time.ParseDuration(cfg.Workload.RampDuration)
	if err != nil {
		return Result{}, fmt.Errorf("parse workload.ramp_duration: %w", err)
	}
	workloadModel, err := workload.NewModel(trace, cfg.Workload.ScaleMW, rampStart, rampDuration, cfg.Workload.DeferrableFraction)
	if err != nil {
		return Result{}, err
	}
	facilityModel, err := facility.New(cfg.Facility.PUEBase, cfg.Facility.CoolingLagSeconds)
	if err != nil {
		return Result{}, err
	}
	thresholds := pmu.EventThresholds{
		FrequencyLowHz: cfg.Controller.EventFrequencyLowHz, FrequencyHighHz: cfg.Controller.EventFrequencyHighHz,
		VoltageLowPU: cfg.Controller.EventVoltageLowPU, VoltageHighPU: cfg.Controller.EventVoltageHighPU,
	}
	events := pmu.DetectEvents(pmuSeries.Samples, thresholds)
	baseline := make([]Point, 0, int(duration/step)+1)
	for ts := start; !ts.After(start.Add(duration)); ts = ts.Add(step) {
		demand := workloadModel.DemandAt(ts, start)
		fac := facilityModel.Step(demand.ITMW, step)
		pmuSample := sampleAt(pmuSeries.Samples, ts)
		baseline = append(baseline, Point{
			Timestamp: ts.UTC(), Mode: ModeBaseline, FrequencyHz: pmuSample.FrequencyHz, VoltagePU: pmuSample.VoltagePU,
			ITMW: demand.ITMW, UrgentMW: demand.UrgentMW, DeferrableMW: demand.DeferrableMW, CoolingMW: fac.CoolingMW,
			FacilityMW: fac.FacilityMW, NetGridMW: fac.FacilityMW, EventActive: isEventActive(events, ts),
		})
	}
	manifest := runs.NewManifest(cfg, pmuSeries)
	now := time.Now().UTC()
	manifest.CreatedAt = &now
	return Result{Config: cfg, Manifest: manifest, Events: events, Baseline: baseline, PMUSummary: pmuSeries.Summary()}, nil
}

func WriteBaselineArtifacts(dir string, result Result) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create run dir %q: %w", dir, err)
	}
	if err := writeJSON(filepath.Join(dir, "manifest.json"), result.Manifest); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(dir, "events.json"), result.Events); err != nil {
		return err
	}
	if err := WritePointsCSV(filepath.Join(dir, "baseline_timeseries.csv"), result.Baseline); err != nil {
		return err
	}
	if len(result.Controlled) > 0 {
		if err := WritePointsCSV(filepath.Join(dir, "controlled_timeseries.csv"), result.Controlled); err != nil {
			return err
		}
		if err := writeJSON(filepath.Join(dir, "metrics.json"), result.Metrics); err != nil {
			return err
		}
	}
	return nil
}

func toMetricPoints(points []Point) []metrics.Point {
	out := make([]metrics.Point, len(points))
	for i, p := range points {
		out[i] = metrics.Point{Timestamp: p.Timestamp, NetGridMW: p.NetGridMW, DeferredQueueMWh: p.DeferredQueueMWh, BESSPowerMW: p.BESSPowerMW, BESSSOC: p.BESSSOC, FrequencyHz: p.FrequencyHz, VoltagePU: p.VoltagePU, EventActive: p.EventActive}
	}
	return out
}

func parseSimulation(cfg scenario.SimulationConfig) (time.Time, time.Duration, time.Duration, error) {
	start, err := time.Parse(time.RFC3339, cfg.Start)
	if err != nil {
		return time.Time{}, 0, 0, fmt.Errorf("parse simulation.start: %w", err)
	}
	duration, err := time.ParseDuration(cfg.Duration)
	if err != nil || duration <= 0 {
		return time.Time{}, 0, 0, fmt.Errorf("simulation.duration must be a positive Go duration")
	}
	step, err := time.ParseDuration(cfg.Step)
	if err != nil || step <= 0 {
		return time.Time{}, 0, 0, fmt.Errorf("simulation.step must be a positive Go duration")
	}
	return start.UTC(), duration, step, nil
}

func loadPMU(cfg scenario.Config) (pmu.Series, error) {
	series, err := pmu.ParseFile(cfg.PMU.File, pmu.ParseOptions{Mapping: pmu.ColumnMapping{
		Timestamp: cfg.PMU.TimestampColumn, FrequencyHz: cfg.PMU.FrequencyColumn, VoltagePU: cfg.PMU.VoltageColumn,
		VoltageAngleDeg: cfg.PMU.VoltageAngleColumn, QualityFlag: cfg.PMU.QualityFlagColumn, SourceID: cfg.PMU.SourceIDColumn,
	}, IgnoreBadQuality: cfg.PMU.IgnoreBadQuality, BadQualityFlags: cfg.PMU.BadQualityFlags})
	if err != nil {
		return pmu.Series{}, err
	}
	if cfg.PMU.ResampleStep != "" {
		step, err := time.ParseDuration(cfg.PMU.ResampleStep)
		if err != nil || step <= 0 {
			return pmu.Series{}, fmt.Errorf("pmu.resample_step must be a positive Go duration")
		}
		resampled, err := pmu.ResampleHold(series.Samples, step)
		if err != nil {
			return pmu.Series{}, err
		}
		series.Samples = resampled
		series.TypicalInterval = step
	}
	return series, nil
}

func sampleAt(samples []pmu.Sample, ts time.Time) pmu.Sample {
	if len(samples) == 0 {
		return pmu.Sample{Timestamp: ts}
	}
	if !ts.After(samples[0].Timestamp) {
		return samples[0]
	}
	for i := len(samples) - 1; i >= 0; i-- {
		if !samples[i].Timestamp.After(ts) {
			return samples[i]
		}
	}
	return samples[0]
}

func isEventActive(events pmu.EventSummary, ts time.Time) bool {
	for _, event := range events.Events {
		end := event.End
		if !ts.Before(event.Start) && (end == nil || ts.Before(*end)) {
			return true
		}
	}
	return false
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", filepath.Base(path), err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
