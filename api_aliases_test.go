package orm

import (
	"context"
	"testing"
)

func TestAliasByPKAndList(t *testing.T) {
	withFreshRegistry(t)
	db := setupSQLiteDB(t)
	ctx := context.Background()

	u1 := crudUser{Email: "a1@example.com", Name: "A1"}
	u2 := crudUser{Email: "a2@example.com", Name: "A2"}
	if err := Insert(ctx, db, &u1); err != nil {
		t.Fatalf("insert u1: %v", err)
	}
	if err := Insert(ctx, db, &u2); err != nil {
		t.Fatalf("insert u2: %v", err)
	}

	got1, err := FindByPK[crudUser](ctx, db, u1.ID)
	if err != nil {
		t.Fatalf("find by pk: %v", err)
	}
	if got1.ID != u1.ID {
		t.Fatalf("unexpected find by pk result: %+v", got1)
	}

	got2, err := GetByPK[crudUser](ctx, db, u2.ID)
	if err != nil {
		t.Fatalf("get by pk: %v", err)
	}
	if got2.ID != u2.ID {
		t.Fatalf("unexpected get by pk result: %+v", got2)
	}

	all, err := FindAll[crudUser](ctx, db)
	if err != nil {
		t.Fatalf("find all: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(all))
	}

	page, err := List[crudUser](ctx, db, 1, 1)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if page.Total != 2 || len(page.Items) != 1 {
		t.Fatalf("unexpected list result: %+v", page)
	}
}

func TestAliasExistsUpdateDeleteAll(t *testing.T) {
	withFreshRegistry(t)
	db := setupSQLiteDB(t)
	ctx := context.Background()

	u1 := crudUser{Email: "e1@example.com", Name: "E1"}
	u2 := crudUser{Email: "e2@example.com", Name: "E2"}
	if err := Insert(ctx, db, &u1); err != nil {
		t.Fatalf("insert u1: %v", err)
	}
	if err := Insert(ctx, db, &u2); err != nil {
		t.Fatalf("insert u2: %v", err)
	}

	exists, err := Exists[crudUser](ctx, db, func(q *ModelQuery[crudUser]) *ModelQuery[crudUser] {
		return q.WhereEq("Email", "e1@example.com")
	})
	if err != nil {
		t.Fatalf("exists: %v", err)
	}
	if !exists {
		t.Fatalf("expected exists=true")
	}

	updated, err := UpdateAll[crudUser](ctx, db, map[string]any{"Name": "UPDATED"}, func(q *ModelQuery[crudUser]) *ModelQuery[crudUser] {
		return q.WhereLike("Email", "e")
	})
	if err != nil {
		t.Fatalf("update all: %v", err)
	}
	if updated != 2 {
		t.Fatalf("expected 2 updated rows, got %d", updated)
	}

	deleted, err := DeleteAll[crudUser](ctx, db, func(q *ModelQuery[crudUser]) *ModelQuery[crudUser] {
		return q.WhereEq("Email", "e1@example.com")
	})
	if err != nil {
		t.Fatalf("delete all: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 deleted row, got %d", deleted)
	}
}
