package scenario

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// Config describes a deterministic grid-response simulation scenario.
//
// Phase 1 keeps the contract JSON-first to avoid dependencies. The fields map
// directly to the YAML shape in the product plan so a future YAML adapter can
// decode into this same struct without changing simulation packages.
type Config struct {
	Name       string           `json:"name"`
	Simulation SimulationConfig `json:"simulation"`
	PMU        PMUConfig        `json:"pmu"`
	Workload   WorkloadConfig   `json:"workload"`
	Facility   FacilityConfig   `json:"facility"`
	BESS       BESSConfig       `json:"bess"`
	Controller ControllerConfig `json:"controller"`
}

type SimulationConfig struct {
	Start    string `json:"start"`
	Duration string `json:"duration"`
	Step     string `json:"step"`
}

type PMUConfig struct {
	File               string   `json:"file"`
	TimestampColumn    string   `json:"timestamp_column,omitempty"`
	FrequencyColumn    string   `json:"frequency_column,omitempty"`
	VoltageColumn      string   `json:"voltage_column,omitempty"`
	VoltageAngleColumn string   `json:"voltage_angle_column,omitempty"`
	QualityFlagColumn  string   `json:"quality_flag_column,omitempty"`
	SourceIDColumn     string   `json:"source_id_column,omitempty"`
	IgnoreBadQuality   bool     `json:"ignore_bad_quality,omitempty"`
	BadQualityFlags    []string `json:"bad_quality_flags,omitempty"`
	ResampleStep       string   `json:"resample_step,omitempty"`
}

type WorkloadConfig struct {
	Source             string  `json:"source"`
	ScaleMW            float64 `json:"scale_mw"`
	RampStart          string  `json:"ramp_start"`
	RampDuration       string  `json:"ramp_duration"`
	DeferrableFraction float64 `json:"deferrable_fraction"`
	UrgencyClass       string  `json:"urgency_class,omitempty"`
}

type FacilityConfig struct {
	CoolingLagSeconds float64 `json:"cooling_lag_seconds"`
	PUEBase           float64 `json:"pue_base"`
}

type BESSConfig struct {
	PowerMW    float64 `json:"power_mw"`
	EnergyMWh  float64 `json:"energy_mwh"`
	InitialSOC float64 `json:"initial_soc"`
	MinSOC     float64 `json:"min_soc"`
	MaxSOC     float64 `json:"max_soc,omitempty"`
}

type ControllerConfig struct {
	RampLimitMWPerMin    float64 `json:"ramp_limit_mw_per_min"`
	RecoveryWindow       string  `json:"recovery_window"`
	EventFrequencyLowHz  float64 `json:"event_frequency_low_hz"`
	EventFrequencyHighHz float64 `json:"event_frequency_high_hz,omitempty"`
	EventVoltageLowPU    float64 `json:"event_voltage_low_pu"`
	EventVoltageHighPU   float64 `json:"event_voltage_high_pu,omitempty"`
}

// ValidationErrors reports all actionable config problems at once.
type ValidationErrors []string

func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return "scenario config is invalid"
	}
	return "scenario config is invalid: " + strings.Join(e, "; ")
}

func LoadJSON(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read scenario config %q: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse scenario config %q as JSON: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	var errs ValidationErrors
	if strings.TrimSpace(c.Name) == "" {
		errs = append(errs, "name is required")
	}
	if _, err := time.Parse(time.RFC3339, c.Simulation.Start); err != nil {
		errs = append(errs, "simulation.start must be RFC3339")
	}
	duration, durationErr := parsePositiveDuration("simulation.duration", c.Simulation.Duration)
	if durationErr != nil {
		errs = append(errs, durationErr.Error())
	}
	step, stepErr := parsePositiveDuration("simulation.step", c.Simulation.Step)
	if stepErr != nil {
		errs = append(errs, stepErr.Error())
	}
	if durationErr == nil && stepErr == nil && duration < step {
		errs = append(errs, "simulation.duration must be >= simulation.step")
	}
	if strings.TrimSpace(c.PMU.File) == "" {
		errs = append(errs, "pmu.file is required")
	}
	if c.PMU.ResampleStep != "" {
		if err := requirePositiveDuration("pmu.resample_step", c.PMU.ResampleStep); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if strings.TrimSpace(c.Workload.Source) == "" {
		errs = append(errs, "workload.source is required")
	}
	if c.Workload.ScaleMW <= 0 {
		errs = append(errs, "workload.scale_mw must be > 0")
	}
	if err := requireNonNegativeDuration("workload.ramp_start", c.Workload.RampStart); err != nil {
		errs = append(errs, err.Error())
	}
	if err := requirePositiveDuration("workload.ramp_duration", c.Workload.RampDuration); err != nil {
		errs = append(errs, err.Error())
	}
	if c.Workload.DeferrableFraction < 0 || c.Workload.DeferrableFraction > 1 {
		errs = append(errs, "workload.deferrable_fraction must be between 0 and 1")
	}
	if c.Facility.CoolingLagSeconds < 0 {
		errs = append(errs, "facility.cooling_lag_seconds must be >= 0")
	}
	if c.Facility.PUEBase < 1 {
		errs = append(errs, "facility.pue_base must be >= 1")
	}
	if c.BESS.PowerMW <= 0 {
		errs = append(errs, "bess.power_mw must be > 0")
	}
	if c.BESS.EnergyMWh <= 0 {
		errs = append(errs, "bess.energy_mwh must be > 0")
	}
	maxSOC := c.BESS.MaxSOC
	if maxSOC == 0 {
		maxSOC = 1
	}
	if c.BESS.MinSOC < 0 || c.BESS.MinSOC > 1 {
		errs = append(errs, "bess.min_soc must be between 0 and 1")
	}
	if maxSOC < 0 || maxSOC > 1 {
		errs = append(errs, "bess.max_soc must be between 0 and 1")
	}
	if c.BESS.MinSOC > maxSOC {
		errs = append(errs, "bess.min_soc must be <= bess.max_soc")
	}
	if c.BESS.InitialSOC < c.BESS.MinSOC || c.BESS.InitialSOC > maxSOC {
		errs = append(errs, "bess.initial_soc must be within min/max SoC")
	}
	if c.Controller.RampLimitMWPerMin <= 0 {
		errs = append(errs, "controller.ramp_limit_mw_per_min must be > 0")
	}
	if err := requirePositiveDuration("controller.recovery_window", c.Controller.RecoveryWindow); err != nil {
		errs = append(errs, err.Error())
	}
	if c.Controller.EventFrequencyLowHz <= 0 && c.Controller.EventFrequencyHighHz <= 0 && c.Controller.EventVoltageLowPU <= 0 && c.Controller.EventVoltageHighPU <= 0 {
		errs = append(errs, "controller must set at least one PMU event threshold")
	}
	if c.Controller.EventFrequencyLowHz > 0 && c.Controller.EventFrequencyHighHz > 0 && c.Controller.EventFrequencyLowHz >= c.Controller.EventFrequencyHighHz {
		errs = append(errs, "controller.event_frequency_low_hz must be < controller.event_frequency_high_hz")
	}
	if c.Controller.EventVoltageLowPU < 0 || c.Controller.EventVoltageHighPU < 0 {
		errs = append(errs, "controller voltage thresholds must be >= 0")
	}
	if c.Controller.EventVoltageLowPU > 0 && c.Controller.EventVoltageHighPU > 0 && c.Controller.EventVoltageLowPU >= c.Controller.EventVoltageHighPU {
		errs = append(errs, "controller.event_voltage_low_pu must be < controller.event_voltage_high_pu")
	}
	if len(errs) > 0 {
		return errs
	}
	return nil
}

func requirePositiveDuration(field, value string) error {
	_, err := parsePositiveDuration(field, value)
	return err
}

func parsePositiveDuration(field, value string) (time.Duration, error) {
	d, err := time.ParseDuration(value)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s must be a positive Go duration", field)
	}
	return d, nil
}

func requireNonNegativeDuration(field, value string) error {
	d, err := time.ParseDuration(value)
	if err != nil || d < 0 {
		return fmt.Errorf("%s must be a non-negative Go duration", field)
	}
	return nil
}
