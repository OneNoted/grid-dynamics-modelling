package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code=%d stderr=%s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"griddyn", "validate-pmu", "validate-scenario", "run"} {
		if !strings.Contains(out, want) {
			t.Fatalf("help missing %q:\n%s", want, out)
		}
	}
}

func TestValidatePMUHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"validate-pmu", "--help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "Usage: griddyn validate-pmu") || !strings.Contains(stderr.String(), "frequency-column") {
		t.Fatalf("validate-pmu help missing expected flags: %s", stderr.String())
	}
}

func TestUnknownCommandFails(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"nope"}, &stdout, &stderr)
	if code == 0 || !strings.Contains(stderr.String(), `unknown command "nope"`) {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
}

func TestValidatePMUSmoke(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"validate-pmu", "../../data/samples/pmu_event_tiny.csv"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code=%d stderr=%s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"PMU CSV valid", "Detected events: 1", "frequency_low+voltage_low"} {
		if !strings.Contains(out, want) {
			t.Fatalf("validate-pmu output missing %q:\n%s", want, out)
		}
	}
}

func TestValidatePMUColumnMapping(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"validate-pmu", "--timestamp-column", "time", "--frequency-column", "freq", "--voltage-column", "vpu", "../../data/samples/pmu_mapped_tiny.csv"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Detected events: 1") {
		t.Fatalf("expected event output, got %s", stdout.String())
	}
}

func TestValidateScenarioSmoke(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"validate-scenario", "../../scenarios/demo.json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `scenario "gpu-ramp-pmu-event-demo" is valid`) {
		t.Fatalf("unexpected output: %s", stdout.String())
	}
}

func TestRunScenarioWritesBaselineArtifacts(t *testing.T) {
	outDir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir("../.."); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(wd) }()
	var stdout, stderr bytes.Buffer
	code := run([]string{"run", "scenarios/demo.json", "--out", outDir}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "controlled samples written") {
		t.Fatalf("unexpected stdout: %s", stdout.String())
	}
	for _, name := range []string{"manifest.json", "events.json", "baseline_timeseries.csv"} {
		if _, err := os.Stat(filepath.Join(outDir, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
}
