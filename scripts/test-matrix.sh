#!/usr/bin/env bash
set -euo pipefail

echo "[matrix] unit/integration (sqlite default)"
go test ./...

if [[ -n "${ORM_TEST_POSTGRES_DSN:-}" ]]; then
  echo "[matrix] postgres integration enabled"
  go test ./... -run '^TestIntegrationPostgres'
else
  echo "[matrix] skip postgres integration: ORM_TEST_POSTGRES_DSN is empty"
fi

echo "[matrix] benchmark smoke"
go test -run '^$' -bench 'Benchmark(ByPK|CompareByPK)' -benchtime=1x ./...
