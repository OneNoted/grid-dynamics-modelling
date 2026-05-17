package pmu

import (
	"strings"
	"testing"
	"time"
)

func TestParseCSVNormalizedAndOptionalColumns(t *testing.T) {
	csv := `timestamp,frequency_hz,voltage_pu,voltage_angle_deg,quality_flag,source_id
2026-01-01T00:00:00Z,60,1.0,12.5,good,A
`
	series, err := ParseCSV(strings.NewReader(csv), ParseOptions{Mapping: ColumnMapping{VoltageAngleDeg: "voltage_angle_deg", QualityFlag: "quality_flag", SourceID: "source_id"}})
	if err != nil {
		t.Fatalf("ParseCSV returned error: %v", err)
	}
	if got := len(series.Samples); got != 1 {
		t.Fatalf("sample count = %d", got)
	}
	s := series.Samples[0]
	if s.VoltageAngleDeg == nil || *s.VoltageAngleDeg != 12.5 || s.QualityFlag != "good" || s.SourceID != "A" {
		t.Fatalf("optional fields not parsed: %+v", s)
	}
}

func TestParseCSVMappedColumnsAndUTCNormalization(t *testing.T) {
	csv := `time,freq,vpu
2026-01-01T01:00:00+01:00,59.99,0.99
`
	series, err := ParseCSV(strings.NewReader(csv), ParseOptions{Mapping: ColumnMapping{Timestamp: "time", FrequencyHz: "freq", VoltagePU: "vpu"}})
	if err != nil {
		t.Fatalf("ParseCSV returned error: %v", err)
	}
	want := "2026-01-01T00:00:00Z"
	if got := series.Samples[0].Timestamp.Format(time.RFC3339); got != want {
		t.Fatalf("timestamp = %s, want %s", got, want)
	}
}

func TestParseCSVRejectsMissingRequiredColumn(t *testing.T) {
	_, err := ParseCSV(strings.NewReader("timestamp,voltage_pu\n2026-01-01T00:00:00Z,1.0\n"), ParseOptions{})
	if err == nil || !strings.Contains(err.Error(), "frequency_hz") {
		t.Fatalf("expected missing frequency_hz error, got %v", err)
	}
}

func TestParseCSVRejectsBadNumericAndDuplicateTimestamp(t *testing.T) {
	_, err := ParseCSV(strings.NewReader("timestamp,frequency_hz,voltage_pu\n2026-01-01T00:00:00Z,NaN,1\n"), ParseOptions{})
	if err == nil || !strings.Contains(err.Error(), "finite") {
		t.Fatalf("expected finite numeric error, got %v", err)
	}
	_, err = ParseCSV(strings.NewReader("timestamp,frequency_hz,voltage_pu\n2026-01-01T00:00:00Z,60,1\n2026-01-01T00:00:00Z,60,1\n"), ParseOptions{})
	if err == nil || !strings.Contains(err.Error(), "duplicate timestamp") {
		t.Fatalf("expected duplicate timestamp error, got %v", err)
	}
}

func TestParseCSVIgnoresBadQualityWhenConfigured(t *testing.T) {
	csv := `timestamp,frequency_hz,voltage_pu,quality_flag
2026-01-01T00:00:00Z,60,1,good
2026-01-01T00:00:01Z,59.90,0.90,bad
2026-01-01T00:00:02Z,60,1,good
`
	series, err := ParseCSV(strings.NewReader(csv), ParseOptions{Mapping: ColumnMapping{QualityFlag: "quality_flag"}, IgnoreBadQuality: true})
	if err != nil {
		t.Fatalf("ParseCSV returned error: %v", err)
	}
	if len(series.Samples) != 2 || series.IgnoredBadQuality != 1 {
		t.Fatalf("samples=%d ignored=%d", len(series.Samples), series.IgnoredBadQuality)
	}
	events := DetectEvents(series.Samples, EventThresholds{FrequencyLowHz: 59.95, VoltageLowPU: 0.95})
	if events.Count != 0 {
		t.Fatalf("bad-quality event should not trigger, got %+v", events)
	}
}

func TestResampleHold(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	samples := []Sample{
		{Timestamp: base, FrequencyHz: 60, VoltagePU: 1},
		{Timestamp: base.Add(2 * time.Second), FrequencyHz: 59.94, VoltagePU: 0.94},
	}
	resampled, err := ResampleHold(samples, time.Second)
	if err != nil {
		t.Fatalf("ResampleHold returned error: %v", err)
	}
	if len(resampled) != 3 {
		t.Fatalf("len(resampled)=%d", len(resampled))
	}
	if resampled[1].FrequencyHz != 60 || resampled[2].FrequencyHz != 59.94 {
		t.Fatalf("unexpected hold values: %+v", resampled)
	}
}

func TestDetectEventsCleanThresholdAndActiveAtEnd(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clean := []Sample{{Timestamp: base, FrequencyHz: 59.95, VoltagePU: 0.95}}
	if got := DetectEvents(clean, EventThresholds{FrequencyLowHz: 59.95, VoltageLowPU: 0.95}); got.Count != 0 {
		t.Fatalf("equal threshold should not trigger: %+v", got)
	}
	samples := []Sample{
		{Timestamp: base, FrequencyHz: 60, VoltagePU: 1},
		{Timestamp: base.Add(time.Second), FrequencyHz: 59.94, VoltagePU: 0.949},
		{Timestamp: base.Add(2 * time.Second), FrequencyHz: 59.93, VoltagePU: 0.948},
	}
	events := DetectEvents(samples, EventThresholds{FrequencyLowHz: 59.95, VoltageLowPU: 0.95})
	if events.Count != 1 {
		t.Fatalf("event count=%d", events.Count)
	}
	event := events.Events[0]
	if event.End != nil {
		t.Fatalf("event should be active at end: %+v", event)
	}
	if event.MinFrequencyHz != 59.93 || event.MinVoltagePU != 0.948 {
		t.Fatalf("wrong extrema: %+v", event)
	}
	if strings.Join(event.Reasons, ",") != "frequency_low,voltage_low" {
		t.Fatalf("wrong reasons: %+v", event.Reasons)
	}
}

func TestDetectEventsHighThresholdsAndDetectionLatency(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	samples := []Sample{
		{Timestamp: base, FrequencyHz: 60.00, VoltagePU: 1.00},
		{Timestamp: base.Add(time.Second), FrequencyHz: 60.06, VoltagePU: 1.06},
		{Timestamp: base.Add(2 * time.Second), FrequencyHz: 60.00, VoltagePU: 1.00},
	}
	latency := 250 * time.Millisecond
	events := DetectEvents(samples, EventThresholds{FrequencyHighHz: 60.05, VoltageHighPU: 1.05, DetectionDelay: latency})
	if events.Count != 1 {
		t.Fatalf("event count=%d events=%+v", events.Count, events.Events)
	}
	event := events.Events[0]
	if event.Start != base.Add(time.Second) {
		t.Fatalf("start=%s", event.Start)
	}
	if event.End == nil || !event.End.Equal(base.Add(2*time.Second)) {
		t.Fatalf("end=%v", event.End)
	}
	if !event.DetectionTime.Equal(base.Add(time.Second).Add(latency)) {
		t.Fatalf("detection time=%s", event.DetectionTime)
	}
	if event.DetectionLatency != latency.String() {
		t.Fatalf("detection latency=%s", event.DetectionLatency)
	}
	if strings.Join(event.Reasons, ",") != "frequency_high,voltage_high" {
		t.Fatalf("wrong reasons: %+v", event.Reasons)
	}
	if event.MaxFrequencyHz != 60.06 || event.MaxVoltagePU != 1.06 {
		t.Fatalf("wrong high extrema: %+v", event)
	}
}

func TestDetectEventsSplitsRecoveredEvents(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	samples := []Sample{
		{Timestamp: base, FrequencyHz: 59.94, VoltagePU: 1},
		{Timestamp: base.Add(time.Second), FrequencyHz: 60, VoltagePU: 1},
		{Timestamp: base.Add(2 * time.Second), FrequencyHz: 59.93, VoltagePU: 1},
		{Timestamp: base.Add(3 * time.Second), FrequencyHz: 60, VoltagePU: 1},
	}
	events := DetectEvents(samples, EventThresholds{FrequencyLowHz: 59.95})
	if events.Count != 2 {
		t.Fatalf("event count=%d events=%+v", events.Count, events.Events)
	}
	if events.Events[0].End == nil || !events.Events[0].End.Equal(base.Add(time.Second)) {
		t.Fatalf("first event end wrong: %+v", events.Events[0])
	}
}
