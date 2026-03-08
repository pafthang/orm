package orm

import (
	"context"
	"testing"
)

func TestUpdateByPKComposite(t *testing.T) {
	withFreshRegistry(t)
	db := setupCompositeDB(t)
	ctx := context.Background()

	row := compositeAccount{TenantID: 7, UserID: 8, Name: "before"}
	if err := Insert(ctx, db, &row); err != nil {
		t.Fatalf("insert: %v", err)
	}
	affected, err := UpdateByPK[compositeAccount](ctx, db, map[string]any{
		"tenant_id": int64(7),
		"user_id":   int64(8),
	}, map[string]any{
		"name": "after",
	})
	if err != nil {
		t.Fatalf("update by pk: %v", err)
	}
	if affected != 1 {
		t.Fatalf("expected affected=1, got %d", affected)
	}
	got, err := ByPK[compositeAccount](ctx, db, map[string]any{"tenant_id": int64(7), "user_id": int64(8)})
	if err != nil {
		t.Fatalf("by pk: %v", err)
	}
	if got.Name != "after" {
		t.Fatalf("expected updated name, got %q", got.Name)
	}
}

func TestUpdateByPKCompositeTuple(t *testing.T) {
	withFreshRegistry(t)
	db := setupCompositeDB(t)
	ctx := context.Background()

	row := compositeAccount{TenantID: 9, UserID: 10, Name: "before"}
	if err := Insert(ctx, db, &row); err != nil {
		t.Fatalf("insert: %v", err)
	}
	affected, err := UpdateByPK[compositeAccount](ctx, db, []any{int64(9), int64(10)}, map[string]any{"name": "after"})
	if err != nil {
		t.Fatalf("update by tuple: %v", err)
	}
	if affected != 1 {
		t.Fatalf("expected affected=1, got %d", affected)
	}
}
