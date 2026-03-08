package orm

import (
	"context"
	"errors"
	"testing"
)

func TestRuntimeMetricsRowsScannedAndTxCount(t *testing.T) {
	withFreshRegistry(t)
	db := setupSQLiteDB(t)
	ctx := context.Background()

	rowsScanned := int64(0)
	txEvents := map[string]int{}
	AttachRuntimeMetrics(RuntimeMetricsOptions{
		OnRowsScanned: func(op Operation, model, table string, rows int64) {
			rowsScanned += rows
		},
		OnTxCount: func(event string) {
			txEvents[event]++
		},
	})
	t.Cleanup(ResetRuntimeMetrics)

	u := crudUser{Email: "rm@example.com", Name: "RuntimeMetrics"}
	if err := Insert(ctx, db, &u); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if _, err := ByPK[crudUser](ctx, db, u.ID); err != nil {
		t.Fatalf("by pk: %v", err)
	}
	if _, err := Query[crudUser](db).WhereEq("ID", u.ID).All(ctx); err != nil {
		t.Fatalf("query all: %v", err)
	}

	_ = WithTx(ctx, db, func(tx *Tx) error {
		return nil
	})
	_ = WithTx(ctx, db, func(tx *Tx) error {
		return errors.New("rollback")
	})

	if rowsScanned < 2 {
		t.Fatalf("expected rows scanned callbacks, got %d", rowsScanned)
	}
	if txEvents["begin"] < 2 || txEvents["commit"] < 1 || txEvents["rollback"] < 1 {
		t.Fatalf("unexpected tx events: %+v", txEvents)
	}
}
