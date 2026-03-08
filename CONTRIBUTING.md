# Contributing

## Development Setup

- Go 1.25+
- SQLite driver for local tests (`modernc.org/sqlite`)
- Optional PostgreSQL for integration tests

## Local Checks

Run before opening a PR:

```bash
go test ./...
make bench-assert
```

For PostgreSQL integration tests:

```bash
export ORM_TEST_POSTGRES_DSN='postgres://orm:orm@localhost:5432/orm?sslmode=disable'
go test ./... -run '^TestIntegrationPostgres'
```

## Pull Requests

- Keep changes scoped and focused.
- Add/adjust tests for behavior changes.
- Update docs when API or behavior changes.
- Prefer backward-compatible changes for public API.

## Commit Style

Use clear commit messages with intent and scope.
