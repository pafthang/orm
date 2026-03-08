package orm

import (
	"context"
	"errors"
	"testing"
)

func withCleanInterceptors(t *testing.T) {
	t.Helper()
	ResetInterceptors()
	t.Cleanup(ResetInterceptors)
}

func TestInterceptorsCalledOnCRUD(t *testing.T) {
	withFreshRegistry(t)
	withCleanInterceptors(t)

	ctx := context.Background()
	db := setupSQLiteDB(t)

	var beforeOps []Operation
	var afterOps []Operation
	AddBeforeInterceptor(func(ctx context.Context, info OperationInfo) (context.Context, error) {
		beforeOps = append(beforeOps, info.Operation)
		return ctx, nil
	})
	AddAfterInterceptor(func(ctx context.Context, info OperationInfo, opErr error) error {
		afterOps = append(afterOps, info.Operation)
		return nil
	})

	u := crudUser{Email: "int@example.com", Name: "Int"}
	if err := Insert(ctx, db, &u); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if _, err := ByPK[crudUser](ctx, db, u.ID); err != nil {
		t.Fatalf("bypk: %v", err)
	}
	if err := UpdateFields(ctx, db, &u, "Name"); err != nil {
		t.Fatalf("update fields: %v", err)
	}
	if err := DeleteByPK[crudUser](ctx, db, u.ID); err != nil {
		t.Fatalf("delete by pk: %v", err)
	}

	if len(beforeOps) == 0 || len(afterOps) == 0 {
		t.Fatalf("interceptors were not called")
	}
	if len(beforeOps) != len(afterOps) {
		t.Fatalf("before/after count mismatch: %d vs %d", len(beforeOps), len(afterOps))
	}
}

func TestBeforeInterceptorStopsOperation(t *testing.T) {
	withFreshRegistry(t)
	withCleanInterceptors(t)

	ctx := context.Background()
	db := setupSQLiteDB(t)

	AddBeforeInterceptor(func(ctx context.Context, info OperationInfo) (context.Context, error) {
		if info.Operation == OpInsert {
			return ctx, errors.New("blocked")
		}
		return ctx, nil
	})

	u := crudUser{Email: "blocked@example.com", Name: "Blocked"}
	err := Insert(ctx, db, &u)
	if err == nil {
		t.Fatalf("expected interceptor error")
	}
	if !isCode(err, CodeInvalidQuery) {
		t.Fatalf("expected invalid_query, got %v", err)
	}

	n, err := Count[crudUser](ctx, db)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Fatalf("insert should be blocked")
	}
}

func TestInterceptorReceivesRichOperationInfo(t *testing.T) {
	withFreshRegistry(t)
	withCleanInterceptors(t)
	ctx := context.Background()
	db := setupPreloadDB(t)

	p := RelProfile{Bio: "bio"}
	if err := Insert(ctx, db, &p); err != nil {
		t.Fatalf("insert profile: %v", err)
	}
	u := RelUser{ProfileID: p.ID, Email: "u@example.com"}
	if err := Insert(ctx, db, &u); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	var seen OperationInfo
	AddBeforeInterceptor(func(ctx context.Context, info OperationInfo) (context.Context, error) {
		if info.Operation == OpQueryAll {
			seen = info
		}
		return ctx, nil
	})

	_, err := Query[RelUser](db).
		WhereEq("email", u.Email).
		JoinRelation("Profile").
		Preload("Profile").
		Limit(10).
		Offset(5).
		All(ctx)
	if err != nil {
		t.Fatalf("query all: %v", err)
	}
	if seen.Operation != OpQueryAll || seen.Model != "RelUser" || seen.Table != "rel_users" {
		t.Fatalf("unexpected operation info head: %+v", seen)
	}
	if !seen.HasWhere || seen.Limit != 10 || seen.Offset != 5 {
		t.Fatalf("unexpected where/limit/offset info: %+v", seen)
	}
	if len(seen.Relations) == 0 || seen.Relations[0] != "Profile" {
		t.Fatalf("expected relation metadata in operation info: %+v", seen.Relations)
	}
	if len(seen.Preloads) == 0 || seen.Preloads[0] != "Profile" {
		t.Fatalf("expected preload metadata in operation info: %+v", seen.Preloads)
	}
}
