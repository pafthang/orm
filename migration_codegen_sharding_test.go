package orm

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pafthang/dbx"
)

func TestMigrationRunnerMigrateUp(t *testing.T) {
	withFreshRegistry(t)
	db := setupSQLiteDB(t)
	r := NewMigrationRunner(db)
	err := r.MigrateUp(context.Background(), []Migration{
		{Version: 1, Name: "create_items", UpSQL: `CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT);`},
	})
	if err != nil {
		t.Fatalf("migrate up: %v", err)
	}
}

func TestMigrationRunnerStatusAndDown(t *testing.T) {
	withFreshRegistry(t)
	db := setupSQLiteDB(t)
	r := NewMigrationRunner(db)
	migrations := []Migration{
		{Version: 1, Name: "create_items", UpSQL: `CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT);`, DownSQL: `DROP TABLE items;`},
		{Version: 2, Name: "create_items2", UpSQL: `CREATE TABLE items2 (id INTEGER PRIMARY KEY, name TEXT);`, DownSQL: `DROP TABLE items2;`},
	}
	if err := r.MigrateUp(context.Background(), migrations); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	st, err := r.Status(context.Background(), migrations)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if st.CurrentVersion != 2 {
		t.Fatalf("expected version 2, got %d", st.CurrentVersion)
	}
	if st.Dirty {
		t.Fatalf("expected clean state")
	}
	if err := r.MigrateDown(context.Background(), migrations, 1); err != nil {
		t.Fatalf("migrate down: %v", err)
	}
	st, err = r.Status(context.Background(), migrations)
	if err != nil {
		t.Fatalf("status2: %v", err)
	}
	if st.CurrentVersion != 1 {
		t.Fatalf("expected version 1, got %d", st.CurrentVersion)
	}
}

func TestMigrationRunnerDirtyStateAndClear(t *testing.T) {
	withFreshRegistry(t)
	db := setupSQLiteDB(t)
	r := NewMigrationRunner(db)
	err := r.MigrateUp(context.Background(), []Migration{
		{Version: 1, Name: "bad_sql", UpSQL: `CREATE TABLE broken (`},
	})
	if err == nil {
		t.Fatalf("expected migrate up error")
	}
	st, err := r.Status(context.Background(), nil)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !st.Dirty || st.LastError == "" {
		t.Fatalf("expected dirty state with last_error, got %+v", st)
	}
	if err := r.ClearDirty(context.Background()); err != nil {
		t.Fatalf("clear dirty: %v", err)
	}
	st, err = r.Status(context.Background(), nil)
	if err != nil {
		t.Fatalf("status after clear: %v", err)
	}
	if st.Dirty || st.LastError != "" {
		t.Fatalf("expected clean state, got %+v", st)
	}
}

func TestMigrationRunnerDBLockHeld(t *testing.T) {
	withFreshRegistry(t)
	db := setupSQLiteDB(t)
	r := NewMigrationRunner(db)
	if err := r.ensureTable(context.Background()); err != nil {
		t.Fatalf("ensure tables: %v", err)
	}
	q := db.Update(r.lockTableName(), map[string]any{
		"locked":     true,
		"lock_owner": "other",
	}, dbx.HashExp{"lock_name": r.LockName})
	if _, err := q.Execute(); err != nil {
		t.Fatalf("force lock: %v", err)
	}
	err := r.MigrateUp(context.Background(), []Migration{
		{Version: 1, Name: "create_ok", UpSQL: `CREATE TABLE ok_lock_test (id INTEGER PRIMARY KEY);`},
	})
	if err == nil {
		t.Fatalf("expected lock held error")
	}
}

func TestLoadMigrationsDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "1_init.up.sql"), []byte("CREATE TABLE a(id INT);"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "1_init.down.sql"), []byte("DROP TABLE a;"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadMigrationsDir(dir)
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	if len(m) != 1 || m[0].Version != 1 {
		t.Fatalf("unexpected migrations: %+v", m)
	}
}

func TestGenerateColumnsConst(t *testing.T) {
	meta, err := Meta[crudUser]()
	if err != nil {
		t.Fatalf("meta: %v", err)
	}
	out, err := GenerateColumnsConst(meta)
	if err != nil {
		t.Fatalf("codegen: %v", err)
	}
	if !strings.Contains(out, "crudUserColumns") {
		t.Fatalf("unexpected codegen output: %s", out)
	}
}

func TestWriteColumnsConstFile(t *testing.T) {
	meta, err := Meta[crudUser]()
	if err != nil {
		t.Fatalf("meta: %v", err)
	}
	path := filepath.Join(t.TempDir(), "cols_gen.go")
	if err := WriteColumnsConstFile(path, meta); err != nil {
		t.Fatalf("write columns file: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if !strings.Contains(string(b), "crudUserColumns") {
		t.Fatalf("unexpected file content: %s", string(b))
	}
}

func TestGenerateFromSpec(t *testing.T) {
	cols, err := GenerateColumnsConstFromSpec("User", []string{"email", "id"})
	if err != nil {
		t.Fatalf("columns spec: %v", err)
	}
	if !strings.Contains(cols, "UserColumns") {
		t.Fatalf("unexpected columns spec output: %s", cols)
	}
	repo, err := GenerateRepositoryStubFromSpec("User", "int64")
	if err != nil {
		t.Fatalf("repo spec: %v", err)
	}
	if !strings.Contains(repo, "type UserRepo") {
		t.Fatalf("unexpected repo spec output: %s", repo)
	}
}

type testShardResolver struct{ db DB }

func (r testShardResolver) ResolveShard(ctx context.Context, shardKey string, info OperationInfo) (DB, error) {
	return r.db, nil
}

func TestSelectDBByShardKey(t *testing.T) {
	withFreshRegistry(t)
	db := setupSQLiteDB(t)
	ctx := WithShardKey(context.Background(), "s1")
	out, err := SelectDB(ctx, db, testShardResolver{db: db}, OperationInfo{Operation: OpQueryAll, Model: "crudUser", Table: "users"})
	if err != nil {
		t.Fatalf("select db: %v", err)
	}
	if out == nil {
		t.Fatalf("expected db")
	}
}

func TestRoutedWrappers(t *testing.T) {
	withFreshRegistry(t)
	db := setupSQLiteDB(t)
	ctx := WithShardKey(context.Background(), "s1")
	resolver := testShardResolver{db: db}

	u := crudUser{Email: "routed@example.com", Name: "Routed"}
	if err := InsertRouted(ctx, db, resolver, &u); err != nil {
		t.Fatalf("insert routed: %v", err)
	}
	got, err := ByPKRouted[crudUser](ctx, db, resolver, u.ID)
	if err != nil {
		t.Fatalf("by pk routed: %v", err)
	}
	if got.Email != u.Email {
		t.Fatalf("unexpected routed row: %+v", got)
	}
}

func TestRouterPolicyRequireShard(t *testing.T) {
	withFreshRegistry(t)
	db := setupSQLiteDB(t)
	r := NewRouter(db, testShardResolver{db: db})
	r.Policy.RequireFor = map[Operation]bool{OpByPK: true}
	_, err := r.SelectDB(context.Background(), OperationInfo{Operation: OpByPK, Model: "crudUser", Table: "users"})
	if err == nil {
		t.Fatalf("expected require shard error")
	}
	ctx := WithShardKey(context.Background(), "s1")
	got, err := r.SelectDB(ctx, OperationInfo{Operation: OpByPK, Model: "crudUser", Table: "users"})
	if err != nil || got == nil {
		t.Fatalf("expected resolved db, got err=%v", err)
	}
}

func TestMigrationRunnerPlanAndMigrateTo(t *testing.T) {
	withFreshRegistry(t)
	db := setupSQLiteDB(t)
	r := NewMigrationRunner(db)
	ctx := context.Background()
	migrations := []Migration{
		{Version: 1, Name: "create_a", UpSQL: `CREATE TABLE a (id INTEGER PRIMARY KEY);`, DownSQL: `DROP TABLE a;`},
		{Version: 2, Name: "create_b", UpSQL: `CREATE TABLE b (id INTEGER PRIMARY KEY);`, DownSQL: `DROP TABLE b;`},
	}
	plan, err := r.Plan(ctx, migrations, 2)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if len(plan.Up) != 2 || len(plan.Down) != 0 {
		t.Fatalf("unexpected up plan: %+v", plan)
	}
	if err := r.MigrateTo(ctx, migrations, 2); err != nil {
		t.Fatalf("migrate to up: %v", err)
	}
	plan, err = r.Plan(ctx, migrations, 1)
	if err != nil {
		t.Fatalf("plan down: %v", err)
	}
	if len(plan.Down) != 1 || plan.Down[0].Version != 2 {
		t.Fatalf("unexpected down plan: %+v", plan)
	}
	if err := r.MigrateTo(ctx, migrations, 1); err != nil {
		t.Fatalf("migrate to down: %v", err)
	}
	st, err := r.Status(ctx, migrations)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if st.CurrentVersion != 1 {
		t.Fatalf("expected version 1, got %d", st.CurrentVersion)
	}
}

func TestMigrationRunnerForceVersion(t *testing.T) {
	withFreshRegistry(t)
	db := setupSQLiteDB(t)
	r := NewMigrationRunner(db)
	ctx := context.Background()
	migrations := []Migration{
		{Version: 1, Name: "create_a", UpSQL: `CREATE TABLE a (id INTEGER PRIMARY KEY);`, DownSQL: `DROP TABLE a;`},
		{Version: 2, Name: "create_b", UpSQL: `CREATE TABLE b (id INTEGER PRIMARY KEY);`, DownSQL: `DROP TABLE b;`},
	}
	if err := r.ForceVersion(ctx, migrations, 2); err != nil {
		t.Fatalf("force: %v", err)
	}
	st, err := r.Status(ctx, migrations)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if st.CurrentVersion != 2 || len(st.Applied) != 2 {
		t.Fatalf("unexpected status after force: %+v", st)
	}
	if err := r.ForceVersion(ctx, migrations, 0); err != nil {
		t.Fatalf("force reset: %v", err)
	}
	st, err = r.Status(ctx, migrations)
	if err != nil {
		t.Fatalf("status2: %v", err)
	}
	if st.CurrentVersion != 0 || len(st.Applied) != 0 {
		t.Fatalf("unexpected status after reset: %+v", st)
	}
}

func TestMigrationRunnerValidateDuplicate(t *testing.T) {
	r := NewMigrationRunner(setupSQLiteDB(t))
	err := r.Validate([]Migration{
		{Version: 1, Name: "a", UpSQL: "SELECT 1"},
		{Version: 1, Name: "b", UpSQL: "SELECT 2"},
	})
	if err == nil {
		t.Fatalf("expected duplicate validation error")
	}
}

func TestRunCodegenPipeline(t *testing.T) {
	dir := t.TempDir()
	cfg := CodegenConfig{
		Package:       "generated",
		OutputDir:     dir,
		GenerateIndex: true,
		Models: []CodegenModelSpec{
			{
				Name:               "User",
				Columns:            []string{"id", "email"},
				PKType:             "int64",
				GenerateColumns:    true,
				GenerateRepository: true,
			},
		},
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal cfg: %v", err)
	}
	cfgPath := filepath.Join(dir, "orm.codegen.json")
	if err := os.WriteFile(cfgPath, b, 0o644); err != nil {
		t.Fatalf("write cfg: %v", err)
	}
	if err := RunCodegenPipeline(cfgPath); err != nil {
		t.Fatalf("run pipeline: %v", err)
	}
	check := []string{
		filepath.Join(dir, "user_columns_gen.go"),
		filepath.Join(dir, "user_repo_gen.go"),
		filepath.Join(dir, "orm_codegen_index_gen.go"),
	}
	for _, p := range check {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read generated %s: %v", p, err)
		}
		if !strings.Contains(string(data), "Code generated by ormtool") {
			t.Fatalf("missing header in %s", p)
		}
	}
}

func TestClusterResolverReadWriteAndHealth(t *testing.T) {
	withFreshRegistry(t)
	dbPrimary := setupSQLiteDB(t)
	dbReplica := setupSQLiteDB(t)
	resolver := NewClusterResolver()
	if err := resolver.Register("s1", ShardReplica{Name: "primary", DB: dbPrimary, Weight: 1}); err != nil {
		t.Fatalf("register primary: %v", err)
	}
	if err := resolver.Register("s1", ShardReplica{Name: "replica", DB: dbReplica, Weight: 1, ReadOnly: true}); err != nil {
		t.Fatalf("register replica: %v", err)
	}
	readDB, err := resolver.ResolveShard(context.Background(), "s1", OperationInfo{Operation: OpQueryAll})
	if err != nil {
		t.Fatalf("resolve read: %v", err)
	}
	if readDB != dbReplica {
		t.Fatalf("expected read replica")
	}
	writeDB, err := resolver.ResolveShard(context.Background(), "s1", OperationInfo{Operation: OpInsert})
	if err != nil {
		t.Fatalf("resolve write: %v", err)
	}
	if writeDB != dbPrimary {
		t.Fatalf("expected primary for write")
	}
	if err := resolver.SetHealthy("s1", "replica", false); err != nil {
		t.Fatalf("set unhealthy: %v", err)
	}
	readDB, err = resolver.ResolveShard(context.Background(), "s1", OperationInfo{Operation: OpQueryAll})
	if err != nil {
		t.Fatalf("resolve read after unhealthy: %v", err)
	}
	if readDB != dbPrimary {
		t.Fatalf("expected fallback to primary")
	}
}
