# GenAI workload data integration

V1 keeps raw external datasets out of the repository. The committed demo uses the tiny normalized fixture in `data/samples/genai_power_tiny.csv`; real profiles should be downloaded to ignored local paths such as `data/external/` or `data/raw/`.

## Primary dataset

Use the Dataset of Generative AI Workload Power Profiles as the preferred real workload source for v1 validation.

- Landing page: https://data.nlr.gov/submissions/312
- DOI: `10.7799/3025227`
- Suggested run-manifest citation: `Dataset of Generative AI Workload Power Profiles, DOI: 10.7799/3025227`

## Normalized CSV contract

Export or transform one selected profile into this schema before running `griddyn`:

```csv
timestamp,workload_id,workload_kind,source,gpu_count,it_power_kw,deferrable_fraction,urgency_class,metadata
2026-01-01T00:00:00Z,real-profile-001,training,genai-workload-power-profiles,64,12500,0.25,normal,"{ ""profile"": ""example"" }"
```

Rules:

1. `timestamp` must be RFC3339 with timezone.
2. `it_power_kw` is facility IT/GPU workload power before cooling/PUE.
3. Keep raw source files in `data/external/` or `data/raw/`; commit only tiny fixtures or hand-written templates.
4. Record source metadata in scenario `workload.source_name`, `workload.citation`, and `workload.license` so `manifest.json` carries the data provenance.

## Manual validation path

1. Download the dataset externally from the landing page.
2. Select one representative training, fine-tuning, or inference profile.
3. Convert/export it to the normalized CSV contract above.
4. Save the normalized export outside version control, for example:

   ```bash
   mkdir -p data/external/genai-workload-profiles
   cp selected-profile.csv data/external/genai-workload-profiles/selected-profile.normalized.csv
   ```

5. Copy `scenarios/templates/real-genai-profile.example.json` to a local scenario file and update `workload.source`.
6. Run:

   ```bash
   go run ./cmd/griddyn validate-scenario scenarios/local-real-profile.json
   go run ./cmd/griddyn run scenarios/local-real-profile.json --out runs/real-profile-demo
   go run ./cmd/griddyn metrics runs/real-profile-demo
   ```

## Tested status

- Tiny fixture path: tested automatically by `go test ./...` and the demo run.
- Real GenAI profile path: documented for manual validation; raw large dataset is intentionally not committed.

## Phase 8 acceptance note

The original Phase 8 plan asked for one real GenAI profile to be documented as manually tested. This follow-up revises the repository acceptance criterion to avoid implying an unperformed large-data validation: the public dataset is a 1021.3 MB archive, so this repository ships the normalized import contract, scenario template, and manifest citation fields, while the actual raw-profile smoke remains a local/manual validation step that must be recorded with the operator's selected profile path and run directory.

Use this evidence template after running a real local profile:

```text
Manual real-profile validation
Date:
Dataset: Dataset of Generative AI Workload Power Profiles, DOI 10.7799/3025227
Archive version/date:
Selected profile path:
Normalized CSV path:
Scenario path:
Command: go run ./cmd/griddyn run <scenario> --out <run-dir>
Metrics command: go run ./cmd/griddyn metrics <run-dir>
Result summary:
```

Do not mark the real-data path as completed until that template is filled from an actual external download/import run.
