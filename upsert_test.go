package orm

import (
	"context"
	"testing"
	"time"

	"github.com/pafthang/dbx"
	_ "modernc.org/sqlite"
)

type upsertUser struct {
	ID        int64     `db:"id,pk"`
	Email     string    `db:"email"`
	Name      string    `db:"name"`
	CreatedAt time.Time `db:"created_at,created_at"`
	UpdatedAt time.Time `db:"updated_at,updated_at"`
}

func (upsertUser) TableName() string { return "upsert_users" }

func setupUpsertDB(t *testing.T) *dbx.DB {
	t.Helper()
	db, err := dbx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	schema := `
CREATE TABLE upsert_users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	email TEXT NOT NULL UNIQUE,
	name TEXT NOT NULL,
	created_at TIMESTAMP NOT NULL,
	updated_at TIMESTAMP NOT NULL
);`
	if _, err := db.NewQuery(schema).Execute(); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return db
}

func TestUpsertInsertAndUpdate(t *testing.T) {
	withFreshRegistry(t)
	db := setupUpsertDB(t)
	ctx := context.Background()

	u := upsertUser{Email: "u@example.com", Name: "v1"}
	if err := Upsert(ctx, db, &u, UpsertOptions{ConflictFields: []string{"Email"}}); err != nil {
		t.Fatalf("upsert insert: %v", err)
	}

	row, err := Query[upsertUser](db).WhereEq("Email", "u@example.com").FindOne(ctx)
	if err != nil {
		t.Fatalf("find row after insert: %v", err)
	}
	if row.Name != "v1" {
		t.Fatalf("expected name v1, got %q", row.Name)
	}
	createdAt := row.CreatedAt
	updatedAt := row.UpdatedAt

	u2 := upsertUser{Email: "u@example.com", Name: "v2"}
	if err := Upsert(ctx, db, &u2, UpsertOptions{ConflictFields: []string{"Email"}}); err != nil {
		t.Fatalf("upsert update: %v", err)
	}

	row2, err := Query[upsertUser](db).WhereEq("Email", "u@example.com").FindOne(ctx)
	if err != nil {
		t.Fatalf("find row after update: %v", err)
	}
	if row2.Name != "v2" {
		t.Fatalf("expected name v2, got %q", row2.Name)
	}
	if !row2.CreatedAt.Equal(createdAt) {
		t.Fatalf("created_at should stay unchanged")
	}
	if !row2.UpdatedAt.After(updatedAt) && !row2.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("updated_at should be refreshed")
	}
}

func TestUpsertDoNothing(t *testing.T) {
	withFreshRegistry(t)
	db := setupUpsertDB(t)
	ctx := context.Background()

	u := upsertUser{Email: "dn@example.com", Name: "v1"}
	if err := Upsert(ctx, db, &u, UpsertOptions{ConflictFields: []string{"Email"}}); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	u2 := upsertUser{Email: "dn@example.com", Name: "v2"}
	if err := Upsert(ctx, db, &u2, UpsertOptions{ConflictFields: []string{"Email"}, DoNothing: true}); err != nil {
		t.Fatalf("upsert do nothing: %v", err)
	}

	row, err := Query[upsertUser](db).WhereEq("Email", "dn@example.com").FindOne(ctx)
	if err != nil {
		t.Fatalf("find row: %v", err)
	}
	if row.Name != "v1" {
		t.Fatalf("do nothing should keep original row, got %q", row.Name)
	}
}
