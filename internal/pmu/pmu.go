package pmu

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

const (
	DefaultTimestampColumn = "timestamp"
	DefaultFrequencyColumn = "frequency_hz"
	DefaultVoltageColumn   = "voltage_pu"
)

type ColumnMapping struct {
	Timestamp       string `json:"timestamp_column,omitempty"`
	FrequencyHz     string `json:"frequency_column,omitempty"`
	VoltagePU       string `json:"voltage_column,omitempty"`
	VoltageAngleDeg string `json:"voltage_angle_column,omitempty"`
	QualityFlag     string `json:"quality_flag_column,omitempty"`
	SourceID        string `json:"source_id_column,omitempty"`
}

func (m ColumnMapping) withDefaults() ColumnMapping {
	if strings.TrimSpace(m.Timestamp) == "" {
		m.Timestamp = DefaultTimestampColumn
	}
	if strings.TrimSpace(m.FrequencyHz) == "" {
		m.FrequencyHz = DefaultFrequencyColumn
	}
	if strings.TrimSpace(m.VoltagePU) == "" {
		m.VoltagePU = DefaultVoltageColumn
	}
	m.Timestamp = strings.TrimSpace(m.Timestamp)
	m.FrequencyHz = strings.TrimSpace(m.FrequencyHz)
	m.VoltagePU = strings.TrimSpace(m.VoltagePU)
	m.VoltageAngleDeg = strings.TrimSpace(m.VoltageAngleDeg)
	m.QualityFlag = strings.TrimSpace(m.QualityFlag)
	m.SourceID = strings.TrimSpace(m.SourceID)
	return m
}

type ParseOptions struct {
	Mapping          ColumnMapping
	IgnoreBadQuality bool
	BadQualityFlags  []string
}

type Sample struct {
	Timestamp       time.Time `json:"timestamp"`
	FrequencyHz     float64   `json:"frequency_hz"`
	VoltagePU       float64   `json:"voltage_pu"`
	VoltageAngleDeg *float64  `json:"voltage_angle_deg,omitempty"`
	QualityFlag     string    `json:"quality_flag,omitempty"`
	SourceID        string    `json:"source_id,omitempty"`
}

type Series struct {
	Samples           []Sample      `json:"samples"`
	Columns           []string      `json:"columns"`
	IgnoredBadQuality int           `json:"ignored_bad_quality"`
	Warnings          []string      `json:"warnings,omitempty"`
	TypicalInterval   time.Duration `json:"typical_interval"`
}

type Summary struct {
	Rows              int        `json:"rows"`
	Start             *time.Time `json:"start,omitempty"`
	End               *time.Time `json:"end,omitempty"`
	TypicalInterval   string     `json:"typical_interval,omitempty"`
	IgnoredBadQuality int        `json:"ignored_bad_quality"`
	Warnings          []string   `json:"warnings,omitempty"`
}

func ParseFile(path string, opts ParseOptions) (Series, error) {
	f, err := os.Open(path)
	if err != nil {
		return Series{}, fmt.Errorf("open PMU CSV %q: %w", path, err)
	}
	defer f.Close()
	series, err := ParseCSV(f, opts)
	if err != nil {
		return Series{}, fmt.Errorf("parse PMU CSV %q: %w", path, err)
	}
	return series, nil
}

func ParseCSV(r io.Reader, opts ParseOptions) (Series, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true

	header, err := cr.Read()
	if errors.Is(err, io.EOF) {
		return Series{}, errors.New("CSV is empty; expected header row")
	}
	if err != nil {
		return Series{}, fmt.Errorf("read header: %w", err)
	}
	indexes, columns, err := buildIndex(header, opts.Mapping.withDefaults())
	if err != nil {
		return Series{}, err
	}

	badFlags := badQualitySet(opts.BadQualityFlags)
	var samples []Sample
	var ignored int
	line := 1
	for {
		record, err := cr.Read()
		line++
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Series{}, fmt.Errorf("line %d: read record: %w", line, err)
		}
		quality := optional(record, indexes.quality)
		if opts.IgnoreBadQuality && quality != "" && badFlags[strings.ToLower(strings.TrimSpace(quality))] {
			ignored++
			continue
		}
		sample, err := parseRecord(record, indexes, line)
		if err != nil {
			return Series{}, err
		}
		samples = append(samples, sample)
	}
	if len(samples) == 0 {
		if ignored > 0 {
			return Series{}, fmt.Errorf("all %d PMU samples were ignored due to bad quality flags", ignored)
		}
		return Series{}, errors.New("CSV contains no PMU samples")
	}
	warnings, err := normalizeSampleOrder(samples)
	if err != nil {
		return Series{}, err
	}
	interval := typicalInterval(samples)
	return Series{Samples: samples, Columns: columns, IgnoredBadQuality: ignored, Warnings: warnings, TypicalInterval: interval}, nil
}

type columnIndexes struct {
	timestamp int
	frequency int
	voltage   int
	angle     int
	quality   int
	source    int
}

func buildIndex(header []string, mapping ColumnMapping) (columnIndexes, []string, error) {
	idx := map[string]int{}
	columns := make([]string, len(header))
	for i, raw := range header {
		name := strings.TrimSpace(strings.TrimPrefix(raw, utf8BOM()))
		columns[i] = name
		if name == "" {
			return columnIndexes{}, nil, fmt.Errorf("header column %d is empty", i+1)
		}
		if _, exists := idx[name]; exists {
			return columnIndexes{}, nil, fmt.Errorf("duplicate CSV column %q", name)
		}
		idx[name] = i
	}
	used := map[string]string{}
	lookupRequired := func(logical, column string) (int, error) {
		i, ok := idx[column]
		if !ok {
			return -1, fmt.Errorf("missing required PMU column %q mapped from %s", column, logical)
		}
		if prev, exists := used[column]; exists {
			return -1, fmt.Errorf("PMU column %q is mapped to both %s and %s", column, prev, logical)
		}
		used[column] = logical
		return i, nil
	}
	lookupOptional := func(logical, column string) (int, error) {
		if column == "" {
			return -1, nil
		}
		i, ok := idx[column]
		if !ok {
			return -1, nil
		}
		if prev, exists := used[column]; exists {
			return -1, fmt.Errorf("PMU column %q is mapped to both %s and %s", column, prev, logical)
		}
		used[column] = logical
		return i, nil
	}
	timestamp, err := lookupRequired("timestamp", mapping.Timestamp)
	if err != nil {
		return columnIndexes{}, nil, err
	}
	frequency, err := lookupRequired("frequency_hz", mapping.FrequencyHz)
	if err != nil {
		return columnIndexes{}, nil, err
	}
	voltage, err := lookupRequired("voltage_pu", mapping.VoltagePU)
	if err != nil {
		return columnIndexes{}, nil, err
	}
	angle, err := lookupOptional("voltage_angle_deg", mapping.VoltageAngleDeg)
	if err != nil {
		return columnIndexes{}, nil, err
	}
	quality, err := lookupOptional("quality_flag", mapping.QualityFlag)
	if err != nil {
		return columnIndexes{}, nil, err
	}
	source, err := lookupOptional("source_id", mapping.SourceID)
	if err != nil {
		return columnIndexes{}, nil, err
	}
	return columnIndexes{timestamp: timestamp, frequency: frequency, voltage: voltage, angle: angle, quality: quality, source: source}, columns, nil
}

func parseRecord(record []string, indexes columnIndexes, line int) (Sample, error) {
	tsRaw := requiredValue(record, indexes.timestamp)
	ts, err := time.Parse(time.RFC3339Nano, tsRaw)
	if err != nil {
		return Sample{}, fmt.Errorf("line %d: timestamp %q must be RFC3339 with timezone: %w", line, tsRaw, err)
	}
	freq, err := parsePositiveFloat(record, indexes.frequency, "frequency_hz", line)
	if err != nil {
		return Sample{}, err
	}
	volt, err := parsePositiveFloat(record, indexes.voltage, "voltage_pu", line)
	if err != nil {
		return Sample{}, err
	}
	var angle *float64
	if indexes.angle >= 0 && optional(record, indexes.angle) != "" {
		v, err := parseFiniteFloat(optional(record, indexes.angle), "voltage_angle_deg", line)
		if err != nil {
			return Sample{}, err
		}
		angle = &v
	}
	return Sample{
		Timestamp: ts.UTC(), FrequencyHz: freq, VoltagePU: volt, VoltageAngleDeg: angle,
		QualityFlag: optional(record, indexes.quality), SourceID: optional(record, indexes.source),
	}, nil
}

func normalizeSampleOrder(samples []Sample) ([]string, error) {
	warnings := []string{}
	unsorted := false
	for i := 1; i < len(samples); i++ {
		if samples[i].Timestamp.Before(samples[i-1].Timestamp) {
			unsorted = true
			break
		}
	}
	if unsorted {
		sort.SliceStable(samples, func(i, j int) bool { return samples[i].Timestamp.Before(samples[j].Timestamp) })
		warnings = append(warnings, "samples were sorted by timestamp")
	}
	for i := 1; i < len(samples); i++ {
		if samples[i].Timestamp.Equal(samples[i-1].Timestamp) {
			return nil, fmt.Errorf("duplicate timestamp detected: %s", samples[i].Timestamp.Format(time.RFC3339Nano))
		}
	}
	return warnings, nil
}

func ResampleHold(samples []Sample, step time.Duration) ([]Sample, error) {
	if step <= 0 {
		return nil, errors.New("resample step must be positive")
	}
	if len(samples) == 0 {
		return nil, errors.New("cannot resample empty PMU series")
	}
	ordered := append([]Sample(nil), samples...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Timestamp.Before(ordered[j].Timestamp) })
	start, end := ordered[0].Timestamp, ordered[len(ordered)-1].Timestamp
	var out []Sample
	idx := 0
	last := ordered[0]
	for t := start; !t.After(end); t = t.Add(step) {
		bucketEnd := t.Add(step)
		chosen := last
		for idx < len(ordered) && ordered[idx].Timestamp.Before(bucketEnd) {
			chosen = ordered[idx]
			idx++
		}
		last = chosen
		chosen.Timestamp = t
		out = append(out, chosen)
	}
	return out, nil
}

func DetectEvents(samples []Sample, thresholds EventThresholds) EventSummary {
	if len(samples) == 0 {
		return EventSummary{}
	}
	ordered := append([]Sample(nil), samples...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Timestamp.Before(ordered[j].Timestamp) })

	var events []Event
	var current *Event
	for _, sample := range ordered {
		reasons := thresholds.reasons(sample)
		if len(reasons) == 0 {
			if current != nil {
				end := sample.Timestamp
				current.End = &end
				events = append(events, *current)
				current = nil
			}
			continue
		}
		if current == nil {
			detectedAt := sample.Timestamp.Add(thresholds.DetectionDelay)
			current = &Event{
				Start: sample.Timestamp, DetectionTime: detectedAt, DetectionLatency: thresholds.DetectionDelay.String(),
				MinFrequencyHz: sample.FrequencyHz, MaxFrequencyHz: sample.FrequencyHz,
				MinVoltagePU: sample.VoltagePU, MaxVoltagePU: sample.VoltagePU,
				Reasons: append([]string(nil), reasons...),
			}
		} else {
			current.Reasons = mergeReasons(current.Reasons, reasons)
			current.MinFrequencyHz = math.Min(current.MinFrequencyHz, sample.FrequencyHz)
			current.MaxFrequencyHz = math.Max(current.MaxFrequencyHz, sample.FrequencyHz)
			current.MinVoltagePU = math.Min(current.MinVoltagePU, sample.VoltagePU)
			current.MaxVoltagePU = math.Max(current.MaxVoltagePU, sample.VoltagePU)
		}
	}
	if current != nil {
		events = append(events, *current)
	}
	return EventSummary{Events: events, Count: len(events)}
}

type EventThresholds struct {
	FrequencyLowHz  float64
	FrequencyHighHz float64
	VoltageLowPU    float64
	VoltageHighPU   float64
	DetectionDelay  time.Duration
}

type EventSummary struct {
	Count  int     `json:"count"`
	Events []Event `json:"events"`
}

type Event struct {
	Start            time.Time  `json:"start"`
	End              *time.Time `json:"end,omitempty"`
	DetectionTime    time.Time  `json:"detection_time"`
	DetectionLatency string     `json:"detection_latency"`
	Reasons          []string   `json:"reasons"`
	MinFrequencyHz   float64    `json:"min_frequency_hz"`
	MaxFrequencyHz   float64    `json:"max_frequency_hz"`
	MinVoltagePU     float64    `json:"min_voltage_pu"`
	MaxVoltagePU     float64    `json:"max_voltage_pu"`
}

func (t EventThresholds) reasons(s Sample) []string {
	var out []string
	if t.FrequencyLowHz > 0 && s.FrequencyHz < t.FrequencyLowHz {
		out = append(out, "frequency_low")
	}
	if t.FrequencyHighHz > 0 && s.FrequencyHz > t.FrequencyHighHz {
		out = append(out, "frequency_high")
	}
	if t.VoltageLowPU > 0 && s.VoltagePU < t.VoltageLowPU {
		out = append(out, "voltage_low")
	}
	if t.VoltageHighPU > 0 && s.VoltagePU > t.VoltageHighPU {
		out = append(out, "voltage_high")
	}
	return out
}

func (s Series) Summary() Summary {
	summary := Summary{Rows: len(s.Samples), IgnoredBadQuality: s.IgnoredBadQuality, Warnings: append([]string(nil), s.Warnings...)}
	if len(s.Samples) > 0 {
		start, end := s.Samples[0].Timestamp, s.Samples[len(s.Samples)-1].Timestamp
		summary.Start = &start
		summary.End = &end
	}
	if s.TypicalInterval > 0 {
		summary.TypicalInterval = s.TypicalInterval.String()
	}
	return summary
}

func badQualitySet(flags []string) map[string]bool {
	if len(flags) == 0 {
		flags = []string{"bad", "invalid", "1"}
	}
	set := map[string]bool{}
	for _, flag := range flags {
		set[strings.ToLower(strings.TrimSpace(flag))] = true
	}
	return set
}

func parsePositiveFloat(record []string, index int, field string, line int) (float64, error) {
	v, err := parseFiniteFloat(requiredValue(record, index), field, line)
	if err != nil {
		return 0, err
	}
	if v <= 0 {
		return 0, fmt.Errorf("line %d: %s must be > 0", line, field)
	}
	return v, nil
}

func parseFiniteFloat(raw, field string, line int) (float64, error) {
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0, fmt.Errorf("line %d: %s %q is not numeric: %w", line, field, raw, err)
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("line %d: %s must be finite", line, field)
	}
	return v, nil
}

func requiredValue(record []string, index int) string {
	if index < 0 || index >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[index])
}

func optional(record []string, index int) string {
	if index < 0 || index >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[index])
}

func typicalInterval(samples []Sample) time.Duration {
	if len(samples) < 2 {
		return 0
	}
	counts := map[time.Duration]int{}
	var best time.Duration
	for i := 1; i < len(samples); i++ {
		d := samples[i].Timestamp.Sub(samples[i-1].Timestamp)
		if d <= 0 {
			continue
		}
		counts[d]++
		if counts[d] > counts[best] {
			best = d
		}
	}
	return best
}

func mergeReasons(existing, next []string) []string {
	seen := map[string]bool{}
	for _, reason := range existing {
		seen[reason] = true
	}
	out := append([]string(nil), existing...)
	for _, reason := range next {
		if !seen[reason] {
			out = append(out, reason)
			seen[reason] = true
		}
	}
	return out
}

func utf8BOM() string { return string([]byte{0xEF, 0xBB, 0xBF}) }
