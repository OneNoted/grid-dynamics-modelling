package runs

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"grid-dynamics-modelling/internal/pmu"
	"grid-dynamics-modelling/internal/scenario"
)

const ManifestSchemaVersion = "griddyn.run-manifest.v1"

type Manifest struct {
	SchemaVersion string                    `json:"schema_version"`
	ScenarioName  string                    `json:"scenario_name"`
	Simulation    scenario.SimulationConfig `json:"simulation"`
	PMU           PMUInput                  `json:"pmu"`
	Controller    scenario.ControllerConfig `json:"controller"`
	Sources       []DataSource              `json:"sources,omitempty"`
	CreatedAt     *time.Time                `json:"created_at,omitempty"`
}

type PMUInput struct {
	Path          string            `json:"path"`
	ColumnMapping pmu.ColumnMapping `json:"column_mapping"`
	Rows          int               `json:"rows,omitempty"`
	Start         *time.Time        `json:"start,omitempty"`
	End           *time.Time        `json:"end,omitempty"`
}

type DataSource struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Path     string `json:"path,omitempty"`
	Citation string `json:"citation,omitempty"`
	License  string `json:"license,omitempty"`
}

func NewManifest(cfg scenario.Config, series pmu.Series) Manifest {
	summary := series.Summary()
	return Manifest{
		SchemaVersion: ManifestSchemaVersion,
		ScenarioName:  cfg.Name,
		Simulation:    cfg.Simulation,
		PMU: PMUInput{
			Path: cfg.PMU.File,
			ColumnMapping: pmu.ColumnMapping{
				Timestamp:       defaultString(cfg.PMU.TimestampColumn, pmu.DefaultTimestampColumn),
				FrequencyHz:     defaultString(cfg.PMU.FrequencyColumn, pmu.DefaultFrequencyColumn),
				VoltagePU:       defaultString(cfg.PMU.VoltageColumn, pmu.DefaultVoltageColumn),
				VoltageAngleDeg: cfg.PMU.VoltageAngleColumn,
				QualityFlag:     cfg.PMU.QualityFlagColumn,
				SourceID:        cfg.PMU.SourceIDColumn,
			},
			Rows:  summary.Rows,
			Start: summary.Start,
			End:   summary.End,
		},
		Controller: cfg.Controller,
		Sources: []DataSource{
			{Name: "Synthetic PMU tiny fixture", Kind: "pmu", Path: cfg.PMU.File, License: "synthetic fixture committed for tests"},
			workloadSource(cfg),
		},
	}
}

func (m Manifest) WriteJSON(w io.Writer) error {
	if m.SchemaVersion == "" {
		m.SchemaVersion = ManifestSchemaVersion
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal run manifest: %w", err)
	}
	_, err = w.Write(append(data, '\n'))
	return err
}

func workloadSource(cfg scenario.Config) DataSource {
	name := defaultString(cfg.Workload.SourceName, "Synthetic GenAI workload tiny fixture")
	license := defaultString(cfg.Workload.License, "synthetic fixture committed for tests")
	return DataSource{Name: name, Kind: "workload", Path: cfg.Workload.Source, Citation: cfg.Workload.Citation, License: license}
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
