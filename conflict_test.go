package orm

import (
	"context"
	"testing"

	"github.com/pafthang/dbx"
	_ "modernc.org/sqlite"
)

type conflictUser struct {
	ID    int64  `db:"id,pk"`
	Email string `db:"email"`
}

func (conflictUser) TableName() string { return "conflict_users" }

func setupConflictDB(t *testing.T) *dbx.DB {
	t.Helper()
	db, err := dbx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.NewQuery(`CREATE TABLE conflict_users (id INTEGER PRIMARY KEY AUTOINCREMENT, email TEXT NOT NULL UNIQUE);`).Execute(); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return db
}

func TestConflictErrorMapping(t *testing.T) {
	withFreshRegistry(t)
	db := setupConflictDB(t)
	ctx := context.Background()

	u1 := conflictUser{Email: "x@example.com"}
	u2 := conflictUser{Email: "x@example.com"}
	if err := Insert(ctx, db, &u1); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	err := Insert(ctx, db, &u2)
	if err == nil {
		t.Fatalf("expected conflict error")
	}
	if !HasCode(err, CodeConflict) {
		t.Fatalf("expected conflict code, got %v", err)
	}
}
