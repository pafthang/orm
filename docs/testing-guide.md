# Testing Guide

## Unit and Integration (Default)

```bash
go test ./...
```

## Matrix Run

```bash
make test-matrix
```

Matrix script includes:
- default unit/integration suite
- optional PostgreSQL integration subset
- benchmark smoke run

## PostgreSQL Integration Setup

```bash
export ORM_TEST_POSTGRES_DSN='postgres://orm:orm@localhost:5432/orm?sslmode=disable'
make test-matrix
```

Current Postgres integration tests:
- `TestIntegrationPostgresCRUD`
- `TestIntegrationPostgresUpsertAndQuery`
- `TestIntegrationPostgresTxAndBatch`

## CI

Workflow: `.github/workflows/ci.yml`

Jobs:
- `test` (unit + benchmark assertions)
- `postgres-integration`

## Coverage

```bash
go test ./... -cover
```

Detailed profile:

```bash
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

## Recommended Regression Set

Before merge:
1. `go test ./...`
2. `make bench-assert`
3. `make test-matrix`
