package orm

import (
	"context"
	"testing"

	"github.com/pafthang/dbx"
	_ "modernc.org/sqlite"
)

type relTenant struct {
	TenantID int64           `db:"tenant_id,pk"`
	UserID   int64           `db:"user_id,pk"`
	Name     string          `db:"name"`
	Items    []relTenantItem `orm:"rel=has_many,local=TenantID|UserID,foreign=TenantID|UserID"`
}

func (relTenant) TableName() string { return "rel_tenants" }

type relTenantItem struct {
	ID       int64     `db:"id,pk"`
	TenantID int64     `db:"tenant_id"`
	UserID   int64     `db:"user_id"`
	Title    string    `db:"title"`
	Parent   relTenant `orm:"rel=belongs_to,local=TenantID|UserID,foreign=TenantID|UserID"`
}

func (relTenantItem) TableName() string { return "rel_tenant_items" }

func setupCompositeRelationDB(t *testing.T) *dbx.DB {
	t.Helper()
	db, err := dbx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	schema := `
CREATE TABLE rel_tenants (
	tenant_id INTEGER NOT NULL,
	user_id INTEGER NOT NULL,
	name TEXT NOT NULL,
	PRIMARY KEY (tenant_id, user_id)
);
CREATE TABLE rel_tenant_items (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	tenant_id INTEGER NOT NULL,
	user_id INTEGER NOT NULL,
	title TEXT NOT NULL
);`
	if _, err := db.NewQuery(schema).Execute(); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return db
}

func TestCompositeRelationPreload(t *testing.T) {
	withFreshRegistry(t)
	db := setupCompositeRelationDB(t)
	ctx := context.Background()

	parent := relTenant{TenantID: 1, UserID: 2, Name: "p"}
	if err := Insert(ctx, db, &parent); err != nil {
		t.Fatalf("insert parent: %v", err)
	}
	item := relTenantItem{TenantID: 1, UserID: 2, Title: "i1"}
	if err := Insert(ctx, db, &item); err != nil {
		t.Fatalf("insert item: %v", err)
	}

	rows, err := Query[relTenant](db).Preload("Items").All(ctx)
	if err != nil {
		t.Fatalf("preload has_many composite: %v", err)
	}
	if len(rows) != 1 || len(rows[0].Items) != 1 {
		t.Fatalf("unexpected preload rows: %+v", rows)
	}

	items, err := Query[relTenantItem](db).Preload("Parent").All(ctx)
	if err != nil {
		t.Fatalf("preload belongs_to composite: %v", err)
	}
	if len(items) != 1 || items[0].Parent.Name != "p" {
		t.Fatalf("unexpected belongs_to preload: %+v", items)
	}
}

func TestCompositeRelationJoin(t *testing.T) {
	withFreshRegistry(t)
	db := setupCompositeRelationDB(t)
	ctx := context.Background()
	p := relTenant{TenantID: 5, UserID: 6, Name: "tenant56"}
	if err := Insert(ctx, db, &p); err != nil {
		t.Fatalf("insert parent: %v", err)
	}
	it := relTenantItem{TenantID: 5, UserID: 6, Title: "x"}
	if err := Insert(ctx, db, &it); err != nil {
		t.Fatalf("insert item: %v", err)
	}
	rows, err := Query[relTenantItem](db).JoinRelation("Parent").WhereRelationEq("Parent.Name", "tenant56").All(ctx)
	if err != nil {
		t.Fatalf("join relation composite: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
}
