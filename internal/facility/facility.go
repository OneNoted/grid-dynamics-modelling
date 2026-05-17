package facility

import (
	"errors"
	"math"
	"time"
)

type Model struct {
	PUEBase           float64
	CoolingLagSeconds float64
	CoolingMW         float64
}

type Step struct {
	ITMW       float64 `json:"it_mw"`
	CoolingMW  float64 `json:"cooling_mw"`
	FacilityMW float64 `json:"facility_mw"`
	PUE        float64 `json:"pue"`
}

func New(pueBase, coolingLagSeconds float64) (Model, error) {
	if pueBase < 1 {
		return Model{}, errors.New("facility PUE base must be >= 1")
	}
	if coolingLagSeconds < 0 {
		return Model{}, errors.New("facility cooling lag seconds must be >= 0")
	}
	return Model{PUEBase: pueBase, CoolingLagSeconds: coolingLagSeconds}, nil
}

func (m *Model) Step(itMW float64, dt time.Duration) Step {
	if itMW < 0 {
		itMW = 0
	}
	targetCooling := itMW * (m.PUEBase - 1)
	if m.CoolingLagSeconds == 0 || dt <= 0 {
		m.CoolingMW = targetCooling
	} else {
		alpha := 1 - math.Exp(-dt.Seconds()/m.CoolingLagSeconds)
		m.CoolingMW += alpha * (targetCooling - m.CoolingMW)
	}
	facility := itMW + m.CoolingMW
	pue := 1.0
	if itMW > 0 {
		pue = facility / itMW
	}
	return Step{ITMW: itMW, CoolingMW: m.CoolingMW, FacilityMW: facility, PUE: pue}
}
