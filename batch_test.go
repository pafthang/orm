package orm

import (
	"context"
	"testing"
)

func TestInsertBatch(t *testing.T) {
	withFreshRegistry(t)
	db := setupSQLiteDB(t)
	ctx := context.Background()

	u1 := &crudUser{Email: "b1@example.com", Name: "B1"}
	u2 := &crudUser{Email: "b2@example.com", Name: "B2"}
	affected, err := InsertBatch(ctx, db, []*crudUser{u1, u2})
	if err != nil {
		t.Fatalf("insert batch: %v", err)
	}
	if affected <= 0 {
		t.Fatalf("expected rows affected > 0, got %d", affected)
	}

	n, err := Count[crudUser](ctx, db)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 rows, got %d", n)
	}
}

func TestInsertBatchEmpty(t *testing.T) {
	withFreshRegistry(t)
	db := setupSQLiteDB(t)
	ctx := context.Background()

	_, err := InsertBatch[crudUser](ctx, db, nil)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !HasCode(err, CodeInvalidQuery) {
		t.Fatalf("expected invalid_query code, got %v", err)
	}
}
