package sim

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
)

func WritePointsCSV(path string, points []Point) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()
	cw := csv.NewWriter(f)
	header := []string{"timestamp", "mode", "frequency_hz", "voltage_pu", "it_mw", "urgent_mw", "deferrable_mw", "cooling_mw", "facility_mw", "bess_power_mw", "bess_soc", "net_grid_mw", "deferred_queue_mwh", "event_active"}
	if err := cw.Write(header); err != nil {
		return fmt.Errorf("write CSV header: %w", err)
	}
	for _, p := range points {
		row := []string{
			p.Timestamp.Format("2006-01-02T15:04:05Z07:00"), string(p.Mode), f64(p.FrequencyHz), f64(p.VoltagePU), f64(p.ITMW), f64(p.UrgentMW), f64(p.DeferrableMW), f64(p.CoolingMW), f64(p.FacilityMW), f64(p.BESSPowerMW), f64(p.BESSSOC), f64(p.NetGridMW), f64(p.DeferredQueueMWh), strconv.FormatBool(p.EventActive),
		}
		if err := cw.Write(row); err != nil {
			return fmt.Errorf("write CSV row: %w", err)
		}
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		return fmt.Errorf("flush CSV: %w", err)
	}
	return nil
}

func f64(v float64) string { return strconv.FormatFloat(v, 'f', 6, 64) }
