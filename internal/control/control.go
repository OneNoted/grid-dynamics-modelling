package control

import (
	"math"
	"time"

	"grid-dynamics-modelling/internal/workload"
)

type Policy struct {
	RampLimitMWPerMin float64
	RecoveryWindow    time.Duration
	recoveryRateMW    float64
}

type Decision struct {
	DeliveredITMW float64 `json:"delivered_it_mw"`
	DeferredMW    float64 `json:"deferred_mw"`
	RecoveredMW   float64 `json:"recovered_mw"`
	BESSRequestMW float64 `json:"bess_request_mw"`
	Action        string  `json:"action"`
}

func New(rampLimitMWPerMin float64, recoveryWindow time.Duration) Policy {
	return Policy{RampLimitMWPerMin: rampLimitMWPerMin, RecoveryWindow: recoveryWindow}
}

func (p *Policy) Decide(demand workload.Demand, queueMWh float64, eventActive bool, dt time.Duration, previousNetMW, rawFacilityMW float64) Decision {
	if eventActive {
		p.recoveryRateMW = 0
		return Decision{DeliveredITMW: demand.UrgentMW, DeferredMW: demand.DeferrableMW, BESSRequestMW: p.BESSRequest(previousNetMW, rawFacilityMW, dt), Action: "defer_deferrable"}
	}
	recoveredMW := 0.0
	if queueMWh > 0 && p.RecoveryWindow > 0 && dt > 0 {
		if p.recoveryRateMW == 0 {
			p.recoveryRateMW = queueMWh / p.RecoveryWindow.Hours()
		}
		recoveredMW = math.Min(queueMWh/dt.Hours(), p.recoveryRateMW)
		if recoveredMW*dt.Hours() >= queueMWh {
			p.recoveryRateMW = 0
		}
	} else {
		p.recoveryRateMW = 0
	}
	deliveredITMW := demand.ITMW + recoveredMW
	return Decision{DeliveredITMW: deliveredITMW, RecoveredMW: recoveredMW, BESSRequestMW: p.BESSRequest(previousNetMW, rawFacilityMW, dt), Action: recoveryAction(recoveredMW)}
}

func (p Policy) BESSRequest(previousNetMW, rawFacilityMW float64, dt time.Duration) float64 {
	if p.RampLimitMWPerMin <= 0 || dt <= 0 {
		return 0
	}
	allowedChange := p.RampLimitMWPerMin * dt.Minutes()
	upper := previousNetMW + allowedChange
	lower := previousNetMW - allowedChange
	if rawFacilityMW > upper {
		return rawFacilityMW - upper
	}
	if rawFacilityMW < lower {
		return rawFacilityMW - lower
	}
	return 0
}

func recoveryAction(recoveredMW float64) string {
	if recoveredMW > 0 {
		return "recover_deferred_work"
	}
	return "none"
}
