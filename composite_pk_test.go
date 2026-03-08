package orm

import (
	"context"
	"testing"

	"github.com/pafthang/dbx"
	_ "modernc.org/sqlite"
)

type compositeAccount struct {
	TenantID int64  `db:"tenant_id,pk"`
	UserID   int64  `db:"user_id,pk"`
	Name     string `db:"name"`
}

func (compositeAccount) TableName() string { return "composite_accounts" }

func setupCompositeDB(t *testing.T) *dbx.DB {
	t.Helper()
	db, err := dbx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	schema := `
CREATE TABLE composite_accounts (
	tenant_id INTEGER NOT NULL,
	user_id INTEGER NOT NULL,
	name TEXT NOT NULL,
	PRIMARY KEY (tenant_id, user_id)
);`
	if _, err := db.NewQuery(schema).Execute(); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return db
}

func TestCompositePKByExistsDelete(t *testing.T) {
	withFreshRegistry(t)
	db := setupCompositeDB(t)
	ctx := context.Background()

	a := compositeAccount{TenantID: 10, UserID: 20, Name: "A"}
	if err := Insert(ctx, db, &a); err != nil {
		t.Fatalf("insert: %v", err)
	}

	type key struct {
		TenantID int64
		UserID   int64
	}
	row, err := ByPK[compositeAccount](ctx, db, key{TenantID: 10, UserID: 20})
	if err != nil {
		t.Fatalf("by pk struct: %v", err)
	}
	if row.Name != "A" {
		t.Fatalf("unexpected row: %+v", row)
	}

	exists, err := ExistsByPK[compositeAccount](ctx, db, map[string]any{"tenant_id": int64(10), "user_id": int64(20)})
	if err != nil {
		t.Fatalf("exists by pk map: %v", err)
	}
	if !exists {
		t.Fatalf("expected row to exist")
	}

	if err := DeleteByPK[compositeAccount](ctx, db, map[string]any{"TenantID": int64(10), "UserID": int64(20)}); err != nil {
		t.Fatalf("delete by pk composite: %v", err)
	}

	exists, err = ExistsByPK[compositeAccount](ctx, db, key{TenantID: 10, UserID: 20})
	if err != nil {
		t.Fatalf("exists after delete: %v", err)
	}
	if exists {
		t.Fatalf("expected row to be deleted")
	}
}

func TestCompositePKUpdateByModel(t *testing.T) {
	withFreshRegistry(t)
	db := setupCompositeDB(t)
	ctx := context.Background()

	a := compositeAccount{TenantID: 1, UserID: 2, Name: "before"}
	if err := Insert(ctx, db, &a); err != nil {
		t.Fatalf("insert: %v", err)
	}
	a.Name = "after"
	if err := Update(ctx, db, &a); err != nil {
		t.Fatalf("update composite by model: %v", err)
	}
	row, err := ByPK[compositeAccount](ctx, db, map[string]any{"tenant_id": int64(1), "user_id": int64(2)})
	if err != nil {
		t.Fatalf("by pk after update: %v", err)
	}
	if row.Name != "after" {
		t.Fatalf("expected updated name, got %q", row.Name)
	}
}

func TestCompositePKByTupleAndTaggedStruct(t *testing.T) {
	withFreshRegistry(t)
	db := setupCompositeDB(t)
	ctx := context.Background()

	a := compositeAccount{TenantID: 3, UserID: 4, Name: "tuple"}
	if err := Insert(ctx, db, &a); err != nil {
		t.Fatalf("insert: %v", err)
	}
	row, err := ByPK[compositeAccount](ctx, db, []any{int64(3), int64(4)})
	if err != nil {
		t.Fatalf("by pk tuple: %v", err)
	}
	if row.Name != "tuple" {
		t.Fatalf("unexpected row: %+v", row)
	}
	type taggedKey struct {
		A int64 `db:"tenant_id"`
		B int64 `db:"user_id"`
	}
	if err := DeleteByPK[compositeAccount](ctx, db, taggedKey{A: 3, B: 4}); err != nil {
		t.Fatalf("delete by tagged key: %v", err)
	}
}
