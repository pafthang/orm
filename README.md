# orm

`orm` is a typed ORM/model layer for Go built on top of [`dbx`](https://github.com/pafthang/dbx).

The package focuses on:
- metadata-driven mapping (`struct` -> table/columns)
- typed CRUD and query builder
- explicit relations and preload
- predictable error model
- hooks/interceptors/plugins
- soft-delete, timestamps, transactions
- observability, routing, migration/codegen scaffolds

## Installation

```bash
go get github.com/pafthang/orm
```

## Requirements

- Go `1.25+`
- Supported drivers depend on `dbx` driver usage
  - SQLite examples use `modernc.org/sqlite`
  - PostgreSQL integration tests use `pgx`

## Quick Example

```go
package main

import (
	"context"
	"log"
	"time"

	"github.com/pafthang/dbx"
	"github.com/pafthang/orm"
	_ "modernc.org/sqlite"
)

type User struct {
	ID        int64      `db:"id,pk"`
	Email     string     `db:"email"`
	Name      string     `db:"name"`
	DeletedAt *time.Time `db:"deleted_at,soft_delete"`
	CreatedAt time.Time  `db:"created_at,created_at"`
	UpdatedAt time.Time  `db:"updated_at,updated_at"`
}

func (User) TableName() string { return "users" }

func main() {
	ctx := context.Background()
	db, err := dbx.Open("sqlite", ":memory:")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	_, _ = db.NewQuery(`
		CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email TEXT NOT NULL,
			name TEXT NOT NULL,
			deleted_at TIMESTAMP NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		);
	`).Execute()

	u := User{Email: "a@example.com", Name: "Alice"}
	if err := orm.Insert(ctx, db, &u); err != nil {
		log.Fatal(err)
	}

	rows, err := orm.Query[User](db).
		WhereEq("email", "a@example.com").
		All(ctx)
	if err != nil {
		log.Fatal(err)
	}
	_ = rows
}
```

## Documentation

- [Quick Start](docs/quick-start.md)
- [Model Guide](docs/model-guide.md)
- [Query Guide](docs/query-guide.md)
- [Relations Guide](docs/relations-guide.md)
- [Hooks Guide](docs/hooks-guide.md)
- [Transactions Guide](docs/transactions-guide.md)
- [DBX Integration](docs/dbx-integration.md)
- [arc Integration](docs/arc-integration.md)
- [Migration Guide](docs/migration-guide.md)
- [Codegen Guide](docs/codegen-guide.md)
- [Sharding Guide](docs/sharding-guide.md)
- [Testing Guide](docs/testing-guide.md)
- [Performance Guide](docs/performance-guide.md)
- [Repository Example](docs/examples/repository.md)

## Current Scope

Implemented:
- metadata/registry/naming
- CRUD/query/preload
- hooks/interceptors/plugins
- soft delete/timestamps
- transactions/savepoints
- upsert/batch/patch
- observability + metrics callbacks
- migration/codegen scaffolds
- migration safety (checksum + db lock + dirty state)
- automatic schema diff generation from models (via snapshot + `ormtool migrate diff`)
- zero-downtime online migration choreography (`expand/backfill/contract`)
- sharding router + routed helpers
- migration lifecycle tooling (plan/validate/goto/force)
- config-driven codegen pipeline
- health-aware weighted shard cluster resolver

Out of scope for now:
- automatic schema conflict resolution for semantic renames
- autonomous deploy orchestration across independent services

## Testing

```bash
go test ./...
make test-matrix
make bench-assert
```

For PostgreSQL integration tests:

```bash
export ORM_TEST_POSTGRES_DSN='postgres://orm:orm@localhost:5432/orm?sslmode=disable'
make test-matrix
```

## CI/CD

- `ci`: unit tests, `go vet`, race tests, PostgreSQL integration tests
- `perf`: benchmark assertions (scheduled/manual and on `main`)
- `release`: triggered by tags `v*`, runs verification and publishes GitHub Release

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md), [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md), and [SECURITY.md](SECURITY.md).

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE).
