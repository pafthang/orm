package orm

import (
	"context"
	"strings"
	"testing"
)

func TestAttachDBXObserverCapturesORMContext(t *testing.T) {
	withFreshRegistry(t)
	db := setupSQLiteDB(t)
	ctx := context.Background()

	events := make([]QueryEvent, 0)
	AttachDBXObserver(db, DBXObserverOptions{
		RedactSQL: func(sql string) string {
			return strings.ReplaceAll(sql, "a@example.com", "[REDACTED]")
		},
		OnEvent: func(e QueryEvent) {
			events = append(events, e)
		},
	})

	u := crudUser{Email: "a@example.com", Name: "Alice"}
	if err := Insert(ctx, db, &u); err != nil {
		t.Fatalf("insert: %v", err)
	}
	_, err := Query[crudUser](db).
		WhereExpr("email = 'a@example.com'", nil).
		All(ctx)
	if err != nil {
		t.Fatalf("query: %v", err)
	}

	if len(events) == 0 {
		t.Fatalf("expected observer events")
	}

	seenInsert := false
	seenQueryAll := false
	for _, e := range events {
		if strings.Contains(e.SQL, "a@example.com") {
			t.Fatalf("sql should be redacted, got %q", e.SQL)
		}
		if e.Operation == OpInsert && e.Model == "crudUser" && e.Table == "users" {
			seenInsert = true
		}
		if e.Operation == OpQueryAll && e.Model == "crudUser" && e.Table == "users" {
			seenQueryAll = true
		}
	}
	if !seenInsert {
		t.Fatalf("expected insert event with orm context")
	}
	if !seenQueryAll {
		t.Fatalf("expected query_all event with orm context")
	}
}
