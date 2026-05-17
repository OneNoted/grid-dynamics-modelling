package workload

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Point is one normalized GenAI workload power sample.
type Point struct {
	Timestamp          time.Time `json:"timestamp"`
	WorkloadID         string    `json:"workload_id"`
	WorkloadKind       string    `json:"workload_kind"`
	Source             string    `json:"source"`
	GPUCount           int       `json:"gpu_count"`
	ITPowerKW          float64   `json:"it_power_kw"`
	DeferrableFraction float64   `json:"deferrable_fraction"`
	UrgencyClass       string    `json:"urgency_class"`
	Metadata           string    `json:"metadata,omitempty"`
}

// Trace is a normalized GenAI power trace. Samples are sorted by timestamp.
type Trace struct {
	Points []Point `json:"points"`
}

type Model struct {
	Trace              Trace
	ScaleMW            float64
	RampStart          time.Duration
	RampDuration       time.Duration
	DeferrableFraction float64
}

type Demand struct {
	Timestamp     time.Time `json:"timestamp"`
	ITMW          float64   `json:"it_mw"`
	UrgentMW      float64   `json:"urgent_mw"`
	DeferrableMW  float64   `json:"deferrable_mw"`
	RampFraction  float64   `json:"ramp_fraction"`
	TraceFraction float64   `json:"trace_fraction"`
}

type Queue struct {
	DeferredMWh float64 `json:"deferred_mwh"`
}

type QueueStep struct {
	ServedDeferrableMW float64 `json:"served_deferrable_mw"`
	DeferredNowMWh     float64 `json:"deferred_now_mwh"`
	RecoveredMWh       float64 `json:"recovered_mwh"`
	RemainingMWh       float64 `json:"remaining_mwh"`
	InfeasibleRecovery bool    `json:"infeasible_recovery"`
}

func LoadFile(path string) (Trace, error) {
	f, err := os.Open(path)
	if err != nil {
		return Trace{}, fmt.Errorf("open workload CSV %q: %w", path, err)
	}
	defer f.Close()
	trace, err := ParseCSV(f)
	if err != nil {
		return Trace{}, fmt.Errorf("parse workload CSV %q: %w", path, err)
	}
	return trace, nil
}

func ParseCSV(r io.Reader) (Trace, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true
	header, err := cr.Read()
	if errors.Is(err, io.EOF) {
		return Trace{}, errors.New("CSV is empty; expected header row")
	}
	if err != nil {
		return Trace{}, fmt.Errorf("read header: %w", err)
	}
	idx, err := workloadIndex(header)
	if err != nil {
		return Trace{}, err
	}
	var points []Point
	line := 1
	for {
		record, err := cr.Read()
		line++
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Trace{}, fmt.Errorf("line %d: read record: %w", line, err)
		}
		point, err := parsePoint(record, idx, line)
		if err != nil {
			return Trace{}, err
		}
		points = append(points, point)
	}
	if len(points) == 0 {
		return Trace{}, errors.New("CSV contains no workload samples")
	}
	sort.SliceStable(points, func(i, j int) bool { return points[i].Timestamp.Before(points[j].Timestamp) })
	return Trace{Points: points}, nil
}

func NewModel(trace Trace, scaleMW float64, rampStart, rampDuration time.Duration, deferrableFraction float64) (Model, error) {
	if len(trace.Points) == 0 {
		return Model{}, errors.New("workload trace requires at least one point")
	}
	if scaleMW <= 0 {
		return Model{}, errors.New("workload scale must be > 0 MW")
	}
	if rampStart < 0 || rampDuration <= 0 {
		return Model{}, errors.New("workload ramp start must be >= 0 and duration must be > 0")
	}
	if deferrableFraction < 0 || deferrableFraction > 1 {
		return Model{}, errors.New("workload deferrable fraction must be between 0 and 1")
	}
	return Model{Trace: trace, ScaleMW: scaleMW, RampStart: rampStart, RampDuration: rampDuration, DeferrableFraction: deferrableFraction}, nil
}

func (m Model) DemandAt(ts time.Time, simStart time.Time) Demand {
	elapsed := ts.Sub(simStart)
	ramp := rampFraction(elapsed, m.RampStart, m.RampDuration)
	traceFraction := m.Trace.FractionAt(ts)
	itMW := m.ScaleMW * ramp * traceFraction
	deferrable := itMW * m.DeferrableFraction
	return Demand{Timestamp: ts.UTC(), ITMW: itMW, UrgentMW: itMW - deferrable, DeferrableMW: deferrable, RampFraction: ramp, TraceFraction: traceFraction}
}

func (t Trace) FractionAt(ts time.Time) float64 {
	if len(t.Points) == 0 {
		return 0
	}
	maxKW := t.MaxKW()
	if maxKW <= 0 {
		return 0
	}
	point := t.PointAt(ts)
	return clamp(point.ITPowerKW/maxKW, 0, 1)
}

func (t Trace) PointAt(ts time.Time) Point {
	points := t.Points
	if len(points) == 0 {
		return Point{}
	}
	if !ts.After(points[0].Timestamp) {
		return points[0]
	}
	for i := len(points) - 1; i >= 0; i-- {
		if !points[i].Timestamp.After(ts) {
			return points[i]
		}
	}
	return points[0]
}

func (t Trace) MaxKW() float64 {
	maxKW := 0.0
	for _, p := range t.Points {
		if p.ITPowerKW > maxKW {
			maxKW = p.ITPowerKW
		}
	}
	return maxKW
}

func (q *Queue) Step(deferrableMW float64, eventActive bool, recoveryCapacityMW float64, dt time.Duration) QueueStep {
	if dt <= 0 {
		return QueueStep{RemainingMWh: q.DeferredMWh}
	}
	hours := dt.Hours()
	if eventActive {
		deferred := nonNegative(deferrableMW) * hours
		q.DeferredMWh += deferred
		return QueueStep{DeferredNowMWh: deferred, RemainingMWh: q.DeferredMWh}
	}
	served := nonNegative(deferrableMW)
	recoveryCapacityMW = nonNegative(recoveryCapacityMW)
	recovered := math.Min(q.DeferredMWh, recoveryCapacityMW*hours)
	q.DeferredMWh -= recovered
	return QueueStep{ServedDeferrableMW: served + recovered/hours, RecoveredMWh: recovered, RemainingMWh: q.DeferredMWh, InfeasibleRecovery: q.DeferredMWh > 1e-9 && recoveryCapacityMW == 0}
}

func rampFraction(elapsed, start, duration time.Duration) float64 {
	if elapsed <= start {
		return 0
	}
	if elapsed >= start+duration {
		return 1
	}
	return elapsed.Seconds()/duration.Seconds() - start.Seconds()/duration.Seconds()
}

type indexes map[string]int

func workloadIndex(header []string) (indexes, error) {
	idx := indexes{}
	for i, raw := range header {
		name := strings.TrimSpace(strings.TrimPrefix(raw, "\ufeff"))
		if name == "" {
			return nil, fmt.Errorf("header column %d is empty", i+1)
		}
		if _, exists := idx[name]; exists {
			return nil, fmt.Errorf("duplicate CSV column %q", name)
		}
		idx[name] = i
	}
	for _, required := range []string{"timestamp", "workload_id", "workload_kind", "source", "gpu_count", "it_power_kw", "deferrable_fraction", "urgency_class"} {
		if _, ok := idx[required]; !ok {
			return nil, fmt.Errorf("missing required workload column %q", required)
		}
	}
	return idx, nil
}

func parsePoint(record []string, idx indexes, line int) (Point, error) {
	ts, err := time.Parse(time.RFC3339Nano, required(record, idx["timestamp"]))
	if err != nil {
		return Point{}, fmt.Errorf("line %d: timestamp must be RFC3339 with timezone: %w", line, err)
	}
	gpu, err := strconv.Atoi(required(record, idx["gpu_count"]))
	if err != nil || gpu < 0 {
		return Point{}, fmt.Errorf("line %d: gpu_count must be a non-negative integer", line)
	}
	powerKW, err := finiteFloat(required(record, idx["it_power_kw"]))
	if err != nil || powerKW < 0 {
		return Point{}, fmt.Errorf("line %d: it_power_kw must be a non-negative finite number", line)
	}
	deferrable, err := finiteFloat(required(record, idx["deferrable_fraction"]))
	if err != nil || deferrable < 0 || deferrable > 1 {
		return Point{}, fmt.Errorf("line %d: deferrable_fraction must be between 0 and 1", line)
	}
	return Point{
		Timestamp: ts.UTC(), WorkloadID: required(record, idx["workload_id"]), WorkloadKind: required(record, idx["workload_kind"]),
		Source: required(record, idx["source"]), GPUCount: gpu, ITPowerKW: powerKW, DeferrableFraction: deferrable,
		UrgencyClass: required(record, idx["urgency_class"]), Metadata: optional(record, idx["metadata"]),
	}, nil
}

func required(record []string, i int) string {
	if i < 0 || i >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[i])
}

func optional(record []string, i int) string {
	if i < 0 || i >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[i])
}

func finiteFloat(raw string) (float64, error) {
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("not finite")
	}
	return v, nil
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func nonNegative(v float64) float64 {
	if v < 0 {
		return 0
	}
	return v
}
