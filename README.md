# grid-dynamics-modelling

`griddyn` is a Go-first simulator scaffold for studying AI datacenter grid-response behavior against PMU-observed disturbances.

Implemented phases 0-2 currently provide:

- Go module and `cmd/griddyn` CLI skeleton.
- JSON scenario contract validation for the v1 PRD fields.
- Tiny synthetic PMU and GenAI workload fixtures under `data/samples/`.
- Run manifest schema primitives under `internal/runs`.
- PMU CSV parsing with configurable column mapping, UTC timestamp normalization, optional quality-flag filtering, hold resampling, and threshold event detection.

## Quickstart

```bash
go test ./...
go run ./cmd/griddyn --help
go run ./cmd/griddyn validate-scenario scenarios/demo.json
go run ./cmd/griddyn validate-pmu data/samples/pmu_event_tiny.csv
```

Mapped PMU columns are supported:

```bash
go run ./cmd/griddyn validate-pmu \
  --timestamp-column time \
  --frequency-column freq \
  --voltage-column vpu \
  data/samples/pmu_mapped_tiny.csv
```

Generated run outputs and large external/raw datasets belong in ignored local paths (`runs/`, `data/raw/`, `data/external/`).
