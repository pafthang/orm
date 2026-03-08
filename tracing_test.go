package orm

import (
	"context"
	"testing"
)

func TestAttachTraceObserver(t *testing.T) {
	withFreshRegistry(t)
	ResetInterceptors()
	t.Cleanup(ResetInterceptors)

	db := setupSQLiteDB(t)
	ctx := context.Background()

	started := 0
	finished := 0

	AttachTraceObserver(TraceObserverOptions{
		OnStart: func(ctx context.Context, info OperationInfo) context.Context {
			started++
			return context.WithValue(ctx, "trace", info.Operation)
		},
		OnFinish: func(ctx context.Context, info OperationInfo, opErr error) {
			finished++
		},
	})

	u := crudUser{Email: "trace@example.com", Name: "Trace"}
	if err := Insert(ctx, db, &u); err != nil {
		t.Fatalf("insert: %v", err)
	}
	_, _ = Query[crudUser](db).WhereExpr("bad_sql( ", nil).All(ctx)

	if started < 2 || finished < 2 {
		t.Fatalf("expected tracing callbacks for both operations, started=%d finished=%d", started, finished)
	}
}
