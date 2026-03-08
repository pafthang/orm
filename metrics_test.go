package orm

import (
	"context"
	"testing"
	"time"
)

func TestAttachMetricsObserver(t *testing.T) {
	withFreshRegistry(t)
	db := setupSQLiteDB(t)
	ctx := context.Background()

	queryCount := 0
	errorCount := 0
	latencyCount := 0
	rowsAffectedCount := 0
	ops := map[Operation]int{}

	AttachMetricsObserver(db, MetricsObserverOptions{
		OnQueryCount: func(kind string, op Operation, model, table string) {
			queryCount++
			ops[op]++
		},
		OnError: func(kind string, op Operation, model, table string, err error) {
			errorCount++
		},
		OnLatency: func(kind string, op Operation, model, table string, d time.Duration) {
			latencyCount++
		},
		OnRowsAffected: func(op Operation, model, table string, rows int64) {
			rowsAffectedCount++
		},
	})

	u := crudUser{Email: "m@example.com", Name: "Metric"}
	if err := Insert(ctx, db, &u); err != nil {
		t.Fatalf("insert: %v", err)
	}
	_, err := Query[crudUser](db).WhereEq("id", u.ID).All(ctx)
	if err != nil {
		t.Fatalf("query all: %v", err)
	}
	u.Name = "Metric2"
	if err := Update(ctx, db, &u); err != nil {
		t.Fatalf("update: %v", err)
	}
	_, _ = Query[crudUser](db).WhereExpr("bad_sql( ", nil).All(ctx)

	if queryCount < 3 {
		t.Fatalf("expected at least 3 query events, got %d", queryCount)
	}
	if latencyCount < 3 {
		t.Fatalf("expected latency callbacks, got %d", latencyCount)
	}
	if errorCount < 1 {
		t.Fatalf("expected at least one error callback, got %d", errorCount)
	}
	if rowsAffectedCount < 1 {
		t.Fatalf("expected rows affected callback on exec, got %d", rowsAffectedCount)
	}
	if ops[OpInsert] == 0 || ops[OpQueryAll] == 0 {
		t.Fatalf("expected operation labels in metrics, got %+v", ops)
	}
}
