package orm

import (
	"context"
	"testing"
)

type tenantUser struct {
	ID       int64  `db:"id,pk"`
	TenantID int64  `db:"tenant_id"`
	Email    string `db:"email"`
}

func (tenantUser) TableName() string { return "tenant_users" }

func setupTenantDB(t *testing.T) DB {
	t.Helper()
	db := setupSQLiteDB(t)
	schema := `
DROP TABLE users;
CREATE TABLE tenant_users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	tenant_id INTEGER NOT NULL,
	email TEXT NOT NULL,
	name TEXT,
	password TEXT,
	deleted_at TIMESTAMP NULL,
	created_at TIMESTAMP,
	updated_at TIMESTAMP
);`
	if _, err := db.NewQuery(schema).Execute(); err != nil {
		t.Fatalf("setup tenant schema: %v", err)
	}
	return db
}

func TestWithTenantFiltersQuery(t *testing.T) {
	withFreshRegistry(t)
	db := setupTenantDB(t)
	ctx := context.Background()

	a := tenantUser{TenantID: 1, Email: "a@t1"}
	b := tenantUser{TenantID: 2, Email: "b@t2"}
	if err := Insert(ctx, db, &a); err != nil {
		t.Fatalf("insert a: %v", err)
	}
	if err := Insert(ctx, db, &b); err != nil {
		t.Fatalf("insert b: %v", err)
	}

	rows, err := Query[tenantUser](db).All(WithTenant(ctx, "tenant_id", int64(1)))
	if err != nil {
		t.Fatalf("tenant query: %v", err)
	}
	if len(rows) != 1 || rows[0].TenantID != 1 {
		t.Fatalf("unexpected tenant rows: %+v", rows)
	}
}

func TestWithTenantValueUsesModelTenantField(t *testing.T) {
	withFreshRegistry(t)
	db := setupTenantDB(t)
	ctx := context.Background()

	a := tenantUser{TenantID: 11, Email: "a@t11"}
	b := tenantUser{TenantID: 22, Email: "b@t22"}
	if err := Insert(ctx, db, &a); err != nil {
		t.Fatalf("insert a: %v", err)
	}
	if err := Insert(ctx, db, &b); err != nil {
		t.Fatalf("insert b: %v", err)
	}
	rows, err := Query[tenantUser](db).All(WithTenantValue(ctx, int64(22)))
	if err != nil {
		t.Fatalf("tenant value query: %v", err)
	}
	if len(rows) != 1 || rows[0].TenantID != 22 {
		t.Fatalf("unexpected rows: %+v", rows)
	}
}

func TestTenantPluginRequired(t *testing.T) {
	withFreshRegistry(t)
	db := setupTenantDB(t)
	ResetPlugins()
	t.Cleanup(ResetPlugins)

	if err := RegisterPlugin(TenantPlugin{Required: true, Column: "tenant_id"}); err != nil {
		t.Fatalf("register plugin: %v", err)
	}
	_, err := Query[tenantUser](db).All(context.Background())
	if err == nil {
		t.Fatalf("expected tenant required error")
	}
	_, err = Query[tenantUser](db).All(WithTenant(context.Background(), "tenant_id", int64(1)))
	if err != nil {
		t.Fatalf("query with tenant context: %v", err)
	}
}

func TestTenantPluginPerModelPolicy(t *testing.T) {
	type secureTenantUser struct {
		ID       int64  `db:"id,pk"`
		TenantID int64  `db:"tenant_id"`
		Email    string `db:"email"`
	}
	withFreshRegistry(t)
	db := setupTenantDB(t)
	ResetPlugins()
	t.Cleanup(ResetPlugins)

	if _, err := DefaultRegistry.RegisterType(secureTenantUser{}, ModelConfig{
		Table:         "tenant_users",
		TenantField:   "tenant_id",
		RequireTenant: boolPtr(true),
	}); err != nil {
		t.Fatalf("register secure model: %v", err)
	}
	if err := RegisterPlugin(TenantPlugin{PerModel: true}); err != nil {
		t.Fatalf("register tenant plugin: %v", err)
	}
	_, err := Query[secureTenantUser](db).All(context.Background())
	if err == nil {
		t.Fatalf("expected per-model tenant required error")
	}
}

func boolPtr(v bool) *bool { return &v }

func TestTenantAppliesToByPKAndCount(t *testing.T) {
	withFreshRegistry(t)
	db := setupTenantDB(t)
	ctx := context.Background()

	a := tenantUser{TenantID: 1, Email: "a@t1"}
	b := tenantUser{TenantID: 2, Email: "b@t2"}
	if err := Insert(ctx, db, &a); err != nil {
		t.Fatalf("insert a: %v", err)
	}
	if err := Insert(ctx, db, &b); err != nil {
		t.Fatalf("insert b: %v", err)
	}

	if _, err := ByPK[tenantUser](WithTenantValue(ctx, int64(2)), db, a.ID); err == nil {
		t.Fatalf("expected not found for wrong tenant")
	}
	n, err := Count[tenantUser](WithTenantValue(ctx, int64(1)), db)
	if err != nil {
		t.Fatalf("count with tenant: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected count=1 for tenant 1, got %d", n)
	}
}
