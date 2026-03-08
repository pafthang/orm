package orm

import (
	"context"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pafthang/dbx"
)

func setupPostgresDB(t *testing.T) *dbx.DB {
	t.Helper()
	dsn := os.Getenv("ORM_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("ORM_TEST_POSTGRES_DSN is not set")
	}
	db, err := dbx.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	schema := `
DROP TABLE IF EXISTS users;
CREATE TABLE users (
	id BIGSERIAL PRIMARY KEY,
	email TEXT NOT NULL UNIQUE,
	name TEXT NOT NULL,
	password TEXT,
	deleted_at TIMESTAMPTZ NULL,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL
);`
	if _, err := db.NewQuery(schema).Execute(); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return db
}

func TestIntegrationPostgresCRUD(t *testing.T) {
	withFreshRegistry(t)
	db := setupPostgresDB(t)
	ctx := context.Background()

	u := crudUser{Email: "pg@example.com", Name: "PG"}
	if err := Insert(ctx, db, &u); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if u.ID == 0 {
		t.Fatalf("expected auto-generated id")
	}

	got, err := ByPK[crudUser](ctx, db, u.ID)
	if err != nil {
		t.Fatalf("by pk: %v", err)
	}
	if got.Email != u.Email {
		t.Fatalf("unexpected row: %+v", got)
	}

	if err := DeleteByPK[crudUser](ctx, db, u.ID); err != nil {
		t.Fatalf("delete by pk: %v", err)
	}
	_, err = ByPK[crudUser](ctx, db, u.ID)
	if err == nil || !HasCode(err, CodeSoftDeleted) {
		t.Fatalf("expected soft-deleted error, got %v", err)
	}
}

func TestIntegrationPostgresUpsertAndQuery(t *testing.T) {
	withFreshRegistry(t)
	db := setupPostgresDB(t)
	ctx := context.Background()

	u := crudUser{Email: "pg-upsert@example.com", Name: "N1"}
	if err := Upsert(ctx, db, &u, UpsertOptions{ConflictFields: []string{"email"}}); err != nil {
		t.Fatalf("upsert insert: %v", err)
	}
	u.Name = "N2"
	if err := Upsert(ctx, db, &u, UpsertOptions{
		ConflictFields: []string{"email"},
		UpdateFields:   []string{"name"},
	}); err != nil {
		t.Fatalf("upsert update: %v", err)
	}
	rows, err := Query[crudUser](db).WhereEq("email", "pg-upsert@example.com").All(ctx)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(rows) != 1 || rows[0].Name != "N2" {
		t.Fatalf("unexpected rows: %+v", rows)
	}
}

func TestIntegrationPostgresTxAndBatch(t *testing.T) {
	withFreshRegistry(t)
	db := setupPostgresDB(t)
	ctx := context.Background()

	err := WithTx(ctx, db, func(tx *Tx) error {
		batch := []*crudUser{
			{Email: "pg-b1@example.com", Name: "B1"},
			{Email: "pg-b2@example.com", Name: "B2"},
		}
		_, err := InsertBatch(ctx, tx, batch)
		return err
	})
	if err != nil {
		t.Fatalf("tx batch: %v", err)
	}
	n, err := Query[crudUser](db).WhereLike("email", "pg-b%@example.com").Count(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 rows, got %d", n)
	}
}
