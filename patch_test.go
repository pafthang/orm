package orm

import (
	"context"
	"testing"

	"github.com/pafthang/dbx"
)

type userPatch struct {
	Name  Optional[string] `db:"name"`
	Email Optional[string] `db:"email"`
}

type nullablePatchModel struct {
	ID   int64   `db:"id,pk"`
	Note *string `db:"note,nullable"`
}

func (nullablePatchModel) TableName() string { return "nullable_patch_models" }

type nullablePatch struct {
	Note Nullable[string] `db:"note"`
}

func setupNullablePatchDB(t *testing.T) *dbx.DB {
	t.Helper()
	db, err := dbx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.NewQuery(`
CREATE TABLE nullable_patch_models (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	note TEXT NULL
);`).Execute(); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return db
}

func TestUpdatePatchByPK(t *testing.T) {
	withFreshRegistry(t)
	db := setupSQLiteDB(t)
	ctx := context.Background()

	u := crudUser{Email: "p@example.com", Name: "Before"}
	if err := Insert(ctx, db, &u); err != nil {
		t.Fatalf("insert: %v", err)
	}

	p := userPatch{Name: Some("After")}
	if err := UpdatePatchByPK[crudUser](ctx, db, u.ID, &p); err != nil {
		t.Fatalf("update patch: %v", err)
	}

	row, err := ByPK[crudUser](ctx, db, u.ID)
	if err != nil {
		t.Fatalf("by pk: %v", err)
	}
	if row.Name != "After" {
		t.Fatalf("expected updated name, got %q", row.Name)
	}
	if row.Email != "p@example.com" {
		t.Fatalf("email should stay unchanged, got %q", row.Email)
	}
}

func TestUpdatePatchByPKEmpty(t *testing.T) {
	withFreshRegistry(t)
	db := setupSQLiteDB(t)
	ctx := context.Background()

	u := crudUser{Email: "p2@example.com", Name: "Before"}
	if err := Insert(ctx, db, &u); err != nil {
		t.Fatalf("insert: %v", err)
	}
	p := userPatch{}
	err := UpdatePatchByPK[crudUser](ctx, db, u.ID, &p)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !HasCode(err, CodeInvalidQuery) {
		t.Fatalf("expected invalid_query code, got %v", err)
	}
}

func TestUpdatePatchByPKNullableNull(t *testing.T) {
	withFreshRegistry(t)
	db := setupNullablePatchDB(t)
	ctx := context.Background()

	val := "note"
	m := nullablePatchModel{Note: &val}
	if err := Insert(ctx, db, &m); err != nil {
		t.Fatalf("insert: %v", err)
	}
	p := nullablePatch{Note: Null[string]()}
	if err := UpdatePatchByPK[nullablePatchModel](ctx, db, m.ID, &p); err != nil {
		t.Fatalf("update patch null: %v", err)
	}
	got, err := ByPK[nullablePatchModel](ctx, db, m.ID)
	if err != nil {
		t.Fatalf("by pk: %v", err)
	}
	if got.Note != nil {
		t.Fatalf("expected nil note after null patch, got %v", *got.Note)
	}
}
