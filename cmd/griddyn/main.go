package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"grid-dynamics-modelling/internal/pmu"
	"grid-dynamics-modelling/internal/scenario"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		printRootHelp(stdout)
		return 0
	}
	cmd := args[0]
	switch cmd {
	case "validate-pmu":
		return runValidatePMU(args[1:], stdout, stderr)
	case "validate-scenario":
		return runValidateScenario(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n", cmd)
		printRootHelp(stderr)
		return 2
	}
}

func printRootHelp(w io.Writer) {
	fmt.Fprintln(w, `griddyn simulates AI datacenter grid-response scenarios.

Usage:
  griddyn --help
  griddyn validate-pmu [flags] <pmu.csv>
  griddyn validate-scenario <scenario.json>

Commands:
  validate-pmu       Validate normalized or mapped PMU CSV and report detected events
  validate-scenario  Validate a JSON scenario config contract

Examples:
  griddyn validate-pmu data/samples/pmu_event_tiny.csv
  griddyn validate-pmu --frequency-column freq --voltage-column vpu data/samples/pmu_mapped_tiny.csv
  griddyn validate-scenario scenarios/demo.json`)
}

func runValidateScenario(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("validate-scenario", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: griddyn validate-scenario <scenario.json>")
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}
	cfg, err := scenario.LoadJSON(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "scenario %q is valid\n", cfg.Name)
	return 0
}

func runValidatePMU(args []string, stdout, stderr io.Writer) int {
	var mapping pmu.ColumnMapping
	var ignoreBad bool
	var badFlags csvList
	var jsonOut bool
	var resampleStep string
	var thresholds pmu.EventThresholds

	fs := flag.NewFlagSet("validate-pmu", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&mapping.Timestamp, "timestamp-column", pmu.DefaultTimestampColumn, "CSV timestamp column")
	fs.StringVar(&mapping.FrequencyHz, "frequency-column", pmu.DefaultFrequencyColumn, "CSV frequency column")
	fs.StringVar(&mapping.VoltagePU, "voltage-column", pmu.DefaultVoltageColumn, "CSV voltage column")
	fs.StringVar(&mapping.VoltageAngleDeg, "voltage-angle-column", "", "optional CSV voltage angle column")
	fs.StringVar(&mapping.QualityFlag, "quality-flag-column", "quality_flag", "optional CSV quality flag column")
	fs.StringVar(&mapping.SourceID, "source-id-column", "source_id", "optional CSV source ID column")
	fs.BoolVar(&ignoreBad, "ignore-bad-quality", false, "ignore samples whose quality flag matches --bad-quality-flag")
	fs.Var(&badFlags, "bad-quality-flag", "bad quality flag value to ignore; may be repeated (default: bad,invalid,1)")
	fs.StringVar(&resampleStep, "resample-step", "", "optional hold resampling step such as 1s")
	fs.Float64Var(&thresholds.FrequencyLowHz, "frequency-low", 59.95, "low frequency event threshold; 0 disables")
	fs.Float64Var(&thresholds.FrequencyHighHz, "frequency-high", 0, "high frequency event threshold; 0 disables")
	fs.Float64Var(&thresholds.VoltageLowPU, "voltage-low", 0.95, "low voltage event threshold; 0 disables")
	fs.Float64Var(&thresholds.VoltageHighPU, "voltage-high", 0, "high voltage event threshold; 0 disables")
	fs.BoolVar(&jsonOut, "json", false, "emit stable JSON summary")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: griddyn validate-pmu [flags] <pmu.csv>")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 || strings.TrimSpace(fs.Arg(0)) == "" {
		fs.Usage()
		return 2
	}

	series, err := pmu.ParseFile(fs.Arg(0), pmu.ParseOptions{Mapping: mapping, IgnoreBadQuality: ignoreBad, BadQualityFlags: badFlags})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if resampleStep != "" {
		step, err := time.ParseDuration(resampleStep)
		if err != nil || step <= 0 {
			fmt.Fprintf(stderr, "--resample-step must be a positive Go duration, got %q\n", resampleStep)
			return 2
		}
		resampled, err := pmu.ResampleHold(series.Samples, step)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		series.Samples = resampled
		series.TypicalInterval = step
		series.Warnings = append(series.Warnings, "samples were resampled with hold policy to "+step.String())
	}
	events := pmu.DetectEvents(series.Samples, thresholds)
	if jsonOut {
		payload := struct {
			Summary pmu.Summary      `json:"summary"`
			Events  pmu.EventSummary `json:"events"`
		}{Summary: series.Summary(), Events: events}
		data, _ := json.MarshalIndent(payload, "", "  ")
		fmt.Fprintln(stdout, string(data))
		return 0
	}
	printPMUValidation(stdout, series, events)
	return 0
}

func printPMUValidation(w io.Writer, series pmu.Series, events pmu.EventSummary) {
	summary := series.Summary()
	fmt.Fprintf(w, "PMU CSV valid: %d rows\n", summary.Rows)
	if summary.Start != nil && summary.End != nil {
		fmt.Fprintf(w, "Time range: %s to %s\n", summary.Start.Format(time.RFC3339Nano), summary.End.Format(time.RFC3339Nano))
	}
	if summary.TypicalInterval != "" {
		fmt.Fprintf(w, "Typical interval: %s\n", summary.TypicalInterval)
	}
	fmt.Fprintf(w, "Columns found: %s\n", strings.Join(series.Columns, ", "))
	fmt.Fprintf(w, "Ignored bad-quality samples: %d\n", summary.IgnoredBadQuality)
	for _, warning := range summary.Warnings {
		fmt.Fprintf(w, "Warning: %s\n", warning)
	}
	fmt.Fprintf(w, "Detected events: %d\n", events.Count)
	for i, event := range events.Events {
		end := "active at end"
		if event.End != nil {
			end = event.End.Format(time.RFC3339Nano)
		}
		fmt.Fprintf(w, "Event %d: start=%s end=%s reasons=%s min_frequency_hz=%.3f min_voltage_pu=%.3f detection_latency=%s\n",
			i+1, event.Start.Format(time.RFC3339Nano), end, strings.Join(event.Reasons, "+"), event.MinFrequencyHz, event.MinVoltagePU, event.DetectionLatency)
	}
}

type csvList []string

func (l *csvList) String() string { return strings.Join(*l, ",") }
func (l *csvList) Set(value string) error {
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			*l = append(*l, part)
		}
	}
	return nil
}
