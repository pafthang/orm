# Migration Guide

## Migration Struct

```go
type Migration struct {
	Version int64
	Name    string
	UpSQL   string
	DownSQL string
}
```

## Runner API

```go
runner := orm.NewMigrationRunner(db)

err := runner.MigrateUp(ctx, migrations)
err = runner.MigrateDown(ctx, migrations, 1)
err = runner.MigrateTo(ctx, migrations, 42)
err = runner.ForceVersion(ctx, migrations, 42)
err = runner.Validate(migrations)
plan, err := runner.Plan(ctx, migrations, 42)
status, err := runner.Status(ctx, migrations)
```

Features:
- ordered apply by version
- ordered rollback by latest applied versions
- target-based upgrade/downgrade (`MigrateTo`)
- planning mode (`Plan`) without execution
- definition validation (`Validate`)
- force-set migration history (`ForceVersion`) for incident recovery
- schema diff generation from model snapshots
- zero-downtime choreography (`expand/backfill/contract`)
- status snapshot (`current/applied/pending`)
- checksum validation of applied migrations
- db-level migration lock (`schema_migrations_lock`)
- dirty state tracking (`schema_migrations_state`)

## Load From Directory

```go
migrations, err := orm.LoadMigrationsDir("./migrations")
```

Filename format:
- `<version>_<name>.up.sql`
- `<version>_<name>.down.sql`

## CLI (`ormtool`)

```bash
go run ./cmd/ormtool migrate status --driver pgx --dsn "$ORM_TEST_POSTGRES_DSN" --dir ./migrations
go run ./cmd/ormtool migrate up --driver pgx --dsn "$ORM_TEST_POSTGRES_DSN" --dir ./migrations
go run ./cmd/ormtool migrate down --driver pgx --dsn "$ORM_TEST_POSTGRES_DSN" --dir ./migrations --steps 1
go run ./cmd/ormtool migrate validate --driver pgx --dsn "$ORM_TEST_POSTGRES_DSN" --dir ./migrations
go run ./cmd/ormtool migrate plan --driver pgx --dsn "$ORM_TEST_POSTGRES_DSN" --dir ./migrations --target 42
go run ./cmd/ormtool migrate goto --driver pgx --dsn "$ORM_TEST_POSTGRES_DSN" --dir ./migrations --target 42
go run ./cmd/ormtool migrate force --driver pgx --dsn "$ORM_TEST_POSTGRES_DSN" --dir ./migrations --target 42
go run ./cmd/ormtool migrate clear-dirty --driver pgx --dsn "$ORM_TEST_POSTGRES_DSN"
go run ./cmd/ormtool migrate diff --driver pgx --dsn "$ORM_TEST_POSTGRES_DSN" --snapshot ./schema.snapshot.json --dir ./migrations --version 100 --name sync_schema
go run ./cmd/ormtool migrate zdt --driver pgx --dsn "$ORM_TEST_POSTGRES_DSN" --snapshot ./schema.snapshot.json --dir ./migrations --version 200 --name zdt_sync
```

## Model Snapshot -> Diff Workflow

Build desired schema snapshot from models in your app code:

```go
snap, err := orm.BuildSchemaSnapshotFromModels(
	User{},
	Order{},
)
if err != nil {
	return err
}
if err := orm.SaveSchemaSnapshot(\"./schema.snapshot.json\", snap); err != nil {
	return err
}
```

Then generate migration SQL:

```bash
go run ./cmd/ormtool migrate diff \
  --driver pgx \
  --dsn "$ORM_TEST_POSTGRES_DSN" \
  --snapshot ./schema.snapshot.json \
  --dir ./migrations \
  --version 100 \
  --name sync_schema
```

For staged rollout:

```bash
go run ./cmd/ormtool migrate zdt \
  --driver pgx \
  --dsn "$ORM_TEST_POSTGRES_DSN" \
  --snapshot ./schema.snapshot.json \
  --dir ./migrations \
  --version 200 \
  --name zdt_sync
```

## Operational Notes

- Keep migrations idempotent where possible.
- Always provide `DownSQL` for reversible deployments.
- Treat checksum mismatch as deployment stop condition.
- If migration fails, state becomes `dirty=true` with `last_error`.
- To recover after manual fix, call `runner.ClearDirty(ctx)` and rerun.
