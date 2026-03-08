package orm

import (
	"context"
	"testing"

	"github.com/pafthang/dbx"
)

func seedUsers(t *testing.T, db DB) []crudUser {
	t.Helper()
	ctx := context.Background()
	users := []crudUser{
		{Email: "a@example.com", Name: "Alice", Password: "p1"},
		{Email: "b@example.com", Name: "Bob", Password: "p2"},
		{Email: "c@example.com", Name: "Carol", Password: "p3"},
	}
	for i := range users {
		if err := Insert(ctx, db, &users[i]); err != nil {
			t.Fatalf("insert seed[%d]: %v", i, err)
		}
	}
	return users
}

func withFreshRegistry(t *testing.T) {
	t.Helper()
	r := NewRegistry()
	old := DefaultRegistry
	DefaultRegistry = r
	t.Cleanup(func() { DefaultRegistry = old })
}

func TestQueryFiltersAndPagination(t *testing.T) {
	withFreshRegistry(t)
	db := setupSQLiteDB(t)
	seedUsers(t, db)
	ctx := context.Background()

	rows, err := Query[crudUser](db).
		WhereLike("Email", "@example.com").
		WhereGTE("ID", int64(2)).
		OrderByDesc("ID").
		Page(1, 2).
		All(ctx)
	if err != nil {
		t.Fatalf("query all: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].ID <= rows[1].ID {
		t.Fatalf("expected desc order by id")
	}
}

func TestQueryOneFirstCountExists(t *testing.T) {
	withFreshRegistry(t)
	db := setupSQLiteDB(t)
	seeded := seedUsers(t, db)
	ctx := context.Background()

	one, err := Query[crudUser](db).WhereEq("ID", seeded[0].ID).One(ctx)
	if err != nil {
		t.Fatalf("one: %v", err)
	}
	if one.ID != seeded[0].ID {
		t.Fatalf("unexpected id: %d", one.ID)
	}

	first, err := Query[crudUser](db).OrderBy("ID").First(ctx)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if first.ID != seeded[0].ID {
		t.Fatalf("unexpected first id: %d", first.ID)
	}

	n, err := Query[crudUser](db).WhereNotEq("Name", "Nobody").Count(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 3 {
		t.Fatalf("expected count=3, got %d", n)
	}

	exists, err := Query[crudUser](db).WhereEq("Email", "b@example.com").Exists(ctx)
	if err != nil {
		t.Fatalf("exists: %v", err)
	}
	if !exists {
		t.Fatalf("expected exists=true")
	}
}

func TestQueryFindOne(t *testing.T) {
	withFreshRegistry(t)
	db := setupSQLiteDB(t)
	seedUsers(t, db)
	ctx := context.Background()

	one, err := Query[crudUser](db).WhereEq("Email", "a@example.com").FindOne(ctx)
	if err != nil {
		t.Fatalf("find one: %v", err)
	}
	if one.Email != "a@example.com" {
		t.Fatalf("unexpected row: %+v", one)
	}

	_, err = Query[crudUser](db).WhereLike("Email", "@example.com").FindOne(ctx)
	if err == nil || !isCode(err, CodeMultipleRows) {
		t.Fatalf("expected multiple rows error, got %v", err)
	}

	_, err = Query[crudUser](db).WhereEq("Email", "none@example.com").FindOne(ctx)
	if err == nil || !isCode(err, CodeNotFound) {
		t.Fatalf("expected not found error, got %v", err)
	}
}

func TestUnsafeWhereExpr(t *testing.T) {
	withFreshRegistry(t)
	db := setupSQLiteDB(t)
	seedUsers(t, db)
	ctx := context.Background()

	rows, err := Query[crudUser](db).
		UnsafeWhereExpr("email LIKE {:p}", dbx.Params{"p": "%@example.com"}).
		All(ctx)
	if err != nil {
		t.Fatalf("unsafe where expr: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
}

func TestQueryWithDeleted(t *testing.T) {
	withFreshRegistry(t)
	db := setupSQLiteDB(t)
	users := seedUsers(t, db)
	ctx := context.Background()

	if err := DeleteByPK[crudUser](ctx, db, users[0].ID); err != nil {
		t.Fatalf("delete by pk: %v", err)
	}

	n, err := Query[crudUser](db).Count(ctx)
	if err != nil {
		t.Fatalf("count without deleted: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 active users, got %d", n)
	}

	nAll, err := Query[crudUser](db).WithDeleted().Count(ctx)
	if err != nil {
		t.Fatalf("count with deleted: %v", err)
	}
	if nAll != 3 {
		t.Fatalf("expected 3 users with deleted, got %d", nAll)
	}
}

func TestQueryOnlyDeleted(t *testing.T) {
	withFreshRegistry(t)
	db := setupSQLiteDB(t)
	users := seedUsers(t, db)
	ctx := context.Background()

	if err := DeleteByPK[crudUser](ctx, db, users[0].ID); err != nil {
		t.Fatalf("delete by pk: %v", err)
	}
	deleted, err := Query[crudUser](db).OnlyDeleted().All(ctx)
	if err != nil {
		t.Fatalf("only deleted query: %v", err)
	}
	if len(deleted) != 1 || deleted[0].ID != users[0].ID {
		t.Fatalf("unexpected only-deleted result: %+v", deleted)
	}
}

func TestQueryInvalidField(t *testing.T) {
	withFreshRegistry(t)
	db := setupSQLiteDB(t)
	seedUsers(t, db)

	_, err := Query[crudUser](db).WhereEq("UnknownField", 1).All(context.Background())
	if err == nil {
		t.Fatalf("expected error")
	}
	if !isCode(err, CodeInvalidField) {
		t.Fatalf("expected invalid field code, got %v", err)
	}
}

func TestQueryList(t *testing.T) {
	withFreshRegistry(t)
	db := setupSQLiteDB(t)
	seedUsers(t, db)
	ctx := context.Background()

	res, err := Query[crudUser](db).
		OrderBy("id").
		List(ctx, 2, 2)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if res.Total != 3 {
		t.Fatalf("expected total=3, got %d", res.Total)
	}
	if res.Page != 2 || res.PerPage != 2 {
		t.Fatalf("unexpected paging: page=%d perPage=%d", res.Page, res.PerPage)
	}
	if len(res.Items) != 1 {
		t.Fatalf("expected 1 item on page 2, got %d", len(res.Items))
	}
}

func TestQueryBulkUpdate(t *testing.T) {
	withFreshRegistry(t)
	db := setupSQLiteDB(t)
	seedUsers(t, db)
	ctx := context.Background()

	rows, err := Query[crudUser](db).
		WhereEq("Name", "Alice").
		Set("Name", "Alice 2").
		Update(ctx)
	if err != nil {
		t.Fatalf("bulk update: %v", err)
	}
	if rows != 1 {
		t.Fatalf("expected 1 updated row, got %d", rows)
	}

	got, err := Query[crudUser](db).WhereEq("Email", "a@example.com").One(ctx)
	if err != nil {
		t.Fatalf("fetch updated: %v", err)
	}
	if got.Name != "Alice 2" {
		t.Fatalf("unexpected updated name: %q", got.Name)
	}
}

func TestQueryDeleteAndHardDelete(t *testing.T) {
	withFreshRegistry(t)
	db := setupSQLiteDB(t)
	seedUsers(t, db)
	ctx := context.Background()

	rows, err := Query[crudUser](db).WhereEq("Email", "b@example.com").Delete(ctx)
	if err != nil {
		t.Fatalf("soft delete by query: %v", err)
	}
	if rows != 1 {
		t.Fatalf("expected 1 soft-deleted row, got %d", rows)
	}

	n, err := Query[crudUser](db).Count(ctx)
	if err != nil {
		t.Fatalf("count after soft delete: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 active rows, got %d", n)
	}

	rows, err = Query[crudUser](db).
		WithDeleted().
		HardDelete().
		WhereEq("Email", "b@example.com").
		Delete(ctx)
	if err != nil {
		t.Fatalf("hard delete by query: %v", err)
	}
	if rows != 1 {
		t.Fatalf("expected 1 hard-deleted row, got %d", rows)
	}

	nAll, err := Query[crudUser](db).WithDeleted().Count(ctx)
	if err != nil {
		t.Fatalf("count with deleted: %v", err)
	}
	if nAll != 2 {
		t.Fatalf("expected 2 remaining rows after hard delete, got %d", nAll)
	}
}
