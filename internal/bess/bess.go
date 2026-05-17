package bess

import (
	"errors"
	"math"
	"time"
)

type Config struct {
	PowerMW    float64
	EnergyMWh  float64
	InitialSOC float64
	MinSOC     float64
	MaxSOC     float64
}

type Battery struct {
	PowerMW   float64 `json:"power_mw"`
	EnergyMWh float64 `json:"energy_mwh"`
	SOC       float64 `json:"soc"`
	MinSOC    float64 `json:"min_soc"`
	MaxSOC    float64 `json:"max_soc"`
}

type Dispatch struct {
	RequestedMW float64 `json:"requested_mw"`
	ActualMW    float64 `json:"actual_mw"`
	UnmetMW     float64 `json:"unmet_mw"`
	SOC         float64 `json:"soc"`
	EnergyMWh   float64 `json:"stored_energy_mwh"`
}

func New(cfg Config) (Battery, error) {
	maxSOC := cfg.MaxSOC
	if maxSOC == 0 {
		maxSOC = 1
	}
	if cfg.PowerMW <= 0 || cfg.EnergyMWh <= 0 {
		return Battery{}, errors.New("BESS power and energy must be > 0")
	}
	if cfg.MinSOC < 0 || cfg.MinSOC > 1 || maxSOC < 0 || maxSOC > 1 || cfg.MinSOC > maxSOC {
		return Battery{}, errors.New("BESS SoC bounds must be between 0 and 1 with min <= max")
	}
	if cfg.InitialSOC < cfg.MinSOC || cfg.InitialSOC > maxSOC {
		return Battery{}, errors.New("BESS initial SoC must be within min/max bounds")
	}
	return Battery{PowerMW: cfg.PowerMW, EnergyMWh: cfg.EnergyMWh, SOC: cfg.InitialSOC, MinSOC: cfg.MinSOC, MaxSOC: maxSOC}, nil
}

// Dispatch applies a signed power request for dt. Positive discharges to reduce
// grid draw; negative charges from the grid. The returned ActualMW uses the same
// sign convention and is clamped by power and SoC constraints.
func (b *Battery) Dispatch(requestMW float64, dt time.Duration) Dispatch {
	if dt <= 0 || requestMW == 0 || b.EnergyMWh <= 0 {
		return b.result(requestMW, 0)
	}
	hours := dt.Hours()
	actual := clamp(requestMW, -b.PowerMW, b.PowerMW)
	stored := b.SOC * b.EnergyMWh
	if actual > 0 {
		available := math.Max(0, stored-b.MinSOC*b.EnergyMWh)
		actual = math.Min(actual, available/hours)
	} else if actual < 0 {
		room := math.Max(0, b.MaxSOC*b.EnergyMWh-stored)
		actual = -math.Min(-actual, room/hours)
	}
	stored -= actual * hours
	b.SOC = clamp(stored/b.EnergyMWh, b.MinSOC, b.MaxSOC)
	return b.result(requestMW, actual)
}

func (b Battery) result(requested, actual float64) Dispatch {
	return Dispatch{RequestedMW: requested, ActualMW: actual, UnmetMW: requested - actual, SOC: b.SOC, EnergyMWh: b.SOC * b.EnergyMWh}
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
