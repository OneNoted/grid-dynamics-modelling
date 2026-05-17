# grid-dynamics-modelling

`griddyn` is a Go-first simulator for studying AI datacenter grid-response behavior against PMU-observed disturbances. It runs an uncontrolled baseline and a controlled response that can defer non-urgent workload and dispatch BESS while reporting ramp-rate, recovery, and ride-through KPIs.

## What is implemented

- Go CLI with `validate-pmu`, `validate-scenario`, `run`, `metrics`, and `serve` commands.
- Scenario validation for the v1 AI datacenter grid-response contract.
- PMU CSV parsing, configurable column mapping, hold resampling, quality filtering, and event detection.
- GenAI workload trace loading, scaling, ramping, deferrable queue, and recovery primitives.
- Reduced-order cooling/PUE facility model.
- BESS power/energy/SoC model.
- Deterministic baseline and controlled simulation artifacts.
- Metrics for ramp-rate comparison, violations, recovery, BESS use, and workload deferral.
- Local HTTP API and dependency-light dashboard assets for KPI cards and charts.
- Real GenAI workload data integration documentation without committing raw large datasets.

## Quickstart from a fresh checkout

```bash
go test ./...
npm --prefix web run build
go run ./cmd/griddyn validate-scenario scenarios/demo.json
go run ./cmd/griddyn validate-pmu data/samples/pmu_event_tiny.csv
go run ./cmd/griddyn run scenarios/demo.json --out runs/demo
go run ./cmd/griddyn metrics runs/demo
go run ./cmd/griddyn serve --run runs/demo
```

Then open <http://127.0.0.1:8080>.

For one-command verification:

```bash
scripts/e2e-smoke.sh
```

The smoke script writes to a temporary run directory by default. Pass a path if you want a stable local output directory:

```bash
scripts/e2e-smoke.sh runs/e2e-demo
```

## Run artifacts

A completed run directory contains:

- `manifest.json` — scenario, source, and data provenance metadata.
- `events.json` — PMU threshold event summary.
- `baseline_timeseries.csv` — uncontrolled facility/grid time series.
- `controlled_timeseries.csv` — controlled time series with BESS/queue fields.
- `metrics.json` — KPI summary used by the CLI and dashboard.

Generated run outputs and large/raw datasets are ignored by default (`runs/`, `data/raw/`, `data/external/`).

## Tiny demo fixtures

Committed fixtures under `data/samples/` are synthetic and intentionally small:

- `pmu_event_tiny.csv` includes a brief voltage/frequency threshold event.
- `pmu_clean_tiny.csv`, `pmu_bad_quality_tiny.csv`, and `pmu_mapped_tiny.csv` cover parser behavior.
- `genai_power_tiny.csv` is a normalized synthetic GenAI IT power trace.

## Real data path

The preferred real workload source is the Dataset of Generative AI Workload Power Profiles:

- Landing page: https://data.nlr.gov/submissions/312
- DOI: `10.7799/3025227`

See `docs/genai-workload-data.md` for download, normalization, citation, and scenario-template guidance. Raw downloads should stay in `data/external/` or `data/raw/` and must not be committed.

## Model assumptions

- Offline deterministic simulation; no real-time control or production scheduler integration.
- Reduced-order facility model: IT load plus first-order cooling lag toward a configured PUE target.
- Rule-based controller: defers configured deferrable workload during detected PMU events, recovers queue over a configured window, and requests BESS dispatch to limit upward grid ramps where BESS is feasible.
- BESS model enforces configured power, energy, min SoC, and max SoC limits.
- No detailed power-flow solver, CFD/HVAC model, MPC, Slurm/Kubernetes integration, or forecasting service in v1.

## Dashboard screenshot placeholders

Dashboard screenshots are not committed yet. Placeholder capture guidance lives in `docs/screenshots/README.md`; capture from the tiny fixture after running `scripts/e2e-smoke.sh`.

## Known limitations

- The committed demo scenario is intentionally tiny and can report infeasible ramp-rate compliance with the configured BESS size; infeasibility is surfaced in `metrics.json` rather than hidden.
- Real GenAI data validation is a documented manual path until a license-safe normalized sample is selected externally.
- The web dashboard is dependency-light and serves checked-in JavaScript directly; `web/src/app.tsx` records the TypeScript data shape for a future bundler if needed.
- YAML scenario parsing is deferred; current scenarios are JSON decoded into the same contract.
