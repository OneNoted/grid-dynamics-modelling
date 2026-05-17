#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RUN_DIR="${1:-$(mktemp -d "${TMPDIR:-/tmp}/griddyn-smoke.XXXXXX")}" 
cd "$ROOT"

echo "== Go tests =="
go test ./...

echo "== Frontend checks =="
npm --prefix web run lint
npm --prefix web run build

echo "== Demo run =="
rm -rf "$RUN_DIR"
go run ./cmd/griddyn run scenarios/demo.json --out "$RUN_DIR"

echo "== Metrics summary =="
go run ./cmd/griddyn metrics "$RUN_DIR"

echo "== Artifact check =="
for artifact in manifest.json events.json baseline_timeseries.csv controlled_timeseries.csv metrics.json; do
  test -s "$RUN_DIR/$artifact"
  echo "ok $artifact"
done

echo "Smoke run complete: $RUN_DIR"
echo "To inspect dashboard: go run ./cmd/griddyn serve --run $RUN_DIR"
