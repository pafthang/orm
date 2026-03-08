package orm

import (
	"context"
	"testing"
	"time"

	"github.com/pafthang/dbx"
	_ "modernc.org/sqlite"
)

type crudUser struct {
	ID        int64      `db:"id,pk"`
	Email     string     `db:"email"`
	Name      string     `db:"name"`
	Password  string     `db:"password,writeonly"`
	DeletedAt *time.Time `db:"deleted_at,soft_delete"`
	CreatedAt time.Time  `db:"created_at,created_at"`
	UpdatedAt time.Time  `db:"updated_at,updated_at"`
}

func (crudUser) TableName() string { return "users" }

func setupSQLiteDB(t *testing.T) *dbx.DB {
	t.Helper()
	db, err := dbx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	schema := `
CREATE TABLE users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	email TEXT NOT NULL,
	name TEXT NOT NULL,
	password TEXT,
	deleted_at TIMESTAMP NULL,
	created_at TIMESTAMP NOT NULL,
	updated_at TIMESTAMP NOT NULL
);`
	if _, err := db.NewQuery(schema).Execute(); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return db
}

func TestCRUDFlow(t *testing.T) {
	ctx := context.Background()
	r := NewRegistry()
	old := DefaultRegistry
	DefaultRegistry = r
	t.Cleanup(func() { DefaultRegistry = old })

	db := setupSQLiteDB(t)

	u := crudUser{Email: "a@example.com", Name: "Alice", Password: "secret"}
	if err := Insert(ctx, db, &u); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if u.ID == 0 {
		t.Fatalf("expected auto id")
	}
	if u.CreatedAt.IsZero() || u.UpdatedAt.IsZero() {
		t.Fatalf("timestamps should be set")
	}

	got, err := ByPK[crudUser](ctx, db, u.ID)
	if err != nil {
		t.Fatalf("by pk: %v", err)
	}
	if got.Email != u.Email || got.Name != u.Name {
		t.Fatalf("unexpected user data: %+v", got)
	}
	if got.Password != "" {
		t.Fatalf("write-only field must not be selected")
	}

	u.Name = "Alice Updated"
	if err := Update(ctx, db, &u); err != nil {
		t.Fatalf("update: %v", err)
	}

	exists, err := ExistsByPK[crudUser](ctx, db, u.ID)
	if err != nil {
		t.Fatalf("exists: %v", err)
	}
	if !exists {
		t.Fatalf("expected row to exist")
	}

	n, err := Count[crudUser](ctx, db)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected count=1, got %d", n)
	}

	if err := Delete(ctx, db, &u); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if u.DeletedAt == nil {
		t.Fatalf("soft delete should set deleted_at")
	}

	n, err = Count[crudUser](ctx, db)
	if err != nil {
		t.Fatalf("count after delete: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected count=0, got %d", n)
	}

	_, err = ByPK[crudUser](ctx, db, u.ID)
	if err == nil || !isCode(err, CodeSoftDeleted) {
		t.Fatalf("expected soft deleted error after soft delete, got %v", err)
	}
}

func TestDeleteByPK(t *testing.T) {
	ctx := context.Background()
	r := NewRegistry()
	old := DefaultRegistry
	DefaultRegistry = r
	t.Cleanup(func() { DefaultRegistry = old })

	db := setupSQLiteDB(t)
	u := crudUser{Email: "b@example.com", Name: "Bob"}
	if err := Insert(ctx, db, &u); err != nil {
		t.Fatalf("insert: %v", err)
	}

	if err := DeleteByPK[crudUser](ctx, db, u.ID); err != nil {
		t.Fatalf("delete by pk: %v", err)
	}

	exists, err := ExistsByPK[crudUser](ctx, db, u.ID)
	if err != nil {
		t.Fatalf("exists: %v", err)
	}
	if exists {
		t.Fatalf("expected deleted row to be hidden")
	}
}

func TestByPKReturnsSoftDeletedError(t *testing.T) {
	ctx := context.Background()
	r := NewRegistry()
	old := DefaultRegistry
	DefaultRegistry = r
	t.Cleanup(func() { DefaultRegistry = old })

	db := setupSQLiteDB(t)
	u := crudUser{Email: "sd@example.com", Name: "SoftDeleted"}
	if err := Insert(ctx, db, &u); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := DeleteByPK[crudUser](ctx, db, u.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err := ByPK[crudUser](ctx, db, u.ID)
	if err == nil || !HasCode(err, CodeSoftDeleted) {
		t.Fatalf("expected soft deleted error, got %v", err)
	}
}

func TestUpdateFields(t *testing.T) {
	ctx := context.Background()
	r := NewRegistry()
	old := DefaultRegistry
	DefaultRegistry = r
	t.Cleanup(func() { DefaultRegistry = old })

	db := setupSQLiteDB(t)
	u := crudUser{Email: "c@example.com", Name: "Carol", Password: "p3"}
	if err := Insert(ctx, db, &u); err != nil {
		t.Fatalf("insert: %v", err)
	}
	beforeUpdatedAt := u.UpdatedAt

	u.Name = "Caroline"
	u.Email = "new@example.com"
	if err := UpdateFields(ctx, db, &u, "Name"); err != nil {
		t.Fatalf("update fields: %v", err)
	}

	got, err := ByPK[crudUser](ctx, db, u.ID)
	if err != nil {
		t.Fatalf("by pk: %v", err)
	}
	if got.Name != "Caroline" {
		t.Fatalf("expected updated name, got %q", got.Name)
	}
	if got.Email != "c@example.com" {
		t.Fatalf("email should stay unchanged, got %q", got.Email)
	}
	if !u.UpdatedAt.After(beforeUpdatedAt) {
		t.Fatalf("updated_at should be refreshed")
	}
}
