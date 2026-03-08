package orm

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pafthang/dbx"
	_ "modernc.org/sqlite"
)

type hookedUser struct {
	ID               int64      `db:"id,pk"`
	Email            string     `db:"email"`
	Name             string     `db:"name"`
	DeletedAt        *time.Time `db:"deleted_at,soft_delete"`
	CreatedAt        time.Time  `db:"created_at,created_at"`
	UpdatedAt        time.Time  `db:"updated_at,updated_at"`
	BeforeInsertN    int        `db:"-"`
	AfterInsertN     int        `db:"-"`
	BeforeUpdateN    int        `db:"-"`
	AfterUpdateN     int        `db:"-"`
	BeforeDeleteN    int        `db:"-"`
	AfterDeleteN     int        `db:"-"`
	AfterFindN       int        `db:"-"`
	FailBeforeInsert bool       `db:"-"`
}

func (hookedUser) TableName() string { return "hooked_users" }

func (u *hookedUser) BeforeInsert(context.Context) error {
	u.BeforeInsertN++
	if u.FailBeforeInsert {
		return errors.New("before insert failed")
	}
	return nil
}
func (u *hookedUser) AfterInsert(context.Context) error  { u.AfterInsertN++; return nil }
func (u *hookedUser) BeforeUpdate(context.Context) error { u.BeforeUpdateN++; return nil }
func (u *hookedUser) AfterUpdate(context.Context) error  { u.AfterUpdateN++; return nil }
func (u *hookedUser) BeforeDelete(context.Context) error { u.BeforeDeleteN++; return nil }
func (u *hookedUser) AfterDelete(context.Context) error  { u.AfterDeleteN++; return nil }
func (u *hookedUser) AfterFind(context.Context) error    { u.AfterFindN++; return nil }

func setupHookDB(t *testing.T) *dbx.DB {
	t.Helper()
	db, err := dbx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	schema := `
CREATE TABLE hooked_users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	email TEXT NOT NULL,
	name TEXT NOT NULL,
	deleted_at TIMESTAMP NULL,
	created_at TIMESTAMP NOT NULL,
	updated_at TIMESTAMP NOT NULL
);`
	if _, err := db.NewQuery(schema).Execute(); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return db
}

func TestLifecycleHooksCRUDAndFind(t *testing.T) {
	ctx := context.Background()
	withFreshRegistry(t)
	db := setupHookDB(t)

	u := hookedUser{Email: "h@example.com", Name: "Hook"}
	if err := Insert(ctx, db, &u); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if u.BeforeInsertN != 1 || u.AfterInsertN != 1 {
		t.Fatalf("insert hooks mismatch: before=%d after=%d", u.BeforeInsertN, u.AfterInsertN)
	}

	u.Name = "Hook 2"
	if err := Update(ctx, db, &u); err != nil {
		t.Fatalf("update: %v", err)
	}
	if u.BeforeUpdateN != 1 || u.AfterUpdateN != 1 {
		t.Fatalf("update hooks mismatch: before=%d after=%d", u.BeforeUpdateN, u.AfterUpdateN)
	}

	fetched, err := ByPK[hookedUser](ctx, db, u.ID)
	if err != nil {
		t.Fatalf("by pk: %v", err)
	}
	if fetched.AfterFindN != 1 {
		t.Fatalf("expected after find hook to run once, got %d", fetched.AfterFindN)
	}

	rows, err := Query[hookedUser](db).WhereEq("id", u.ID).All(ctx)
	if err != nil {
		t.Fatalf("query all: %v", err)
	}
	if len(rows) != 1 || rows[0].AfterFindN != 1 {
		t.Fatalf("expected after find hook in query all")
	}

	if err := Delete(ctx, db, &u); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if u.BeforeDeleteN != 1 || u.AfterDeleteN != 1 {
		t.Fatalf("delete hooks mismatch: before=%d after=%d", u.BeforeDeleteN, u.AfterDeleteN)
	}
}

func TestBeforeInsertHookErrorStopsInsert(t *testing.T) {
	ctx := context.Background()
	withFreshRegistry(t)
	db := setupHookDB(t)

	u := hookedUser{Email: "bad@example.com", Name: "Bad", FailBeforeInsert: true}
	err := Insert(ctx, db, &u)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !isCode(err, CodeInvalidQuery) {
		t.Fatalf("expected invalid_query code, got %v", err)
	}

	n, err := Query[hookedUser](db).WithDeleted().Count(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Fatalf("row should not be inserted when before hook fails")
	}
}
