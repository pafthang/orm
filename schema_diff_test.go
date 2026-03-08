package orm

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type schemaDiffUser struct {
	ID        int64     `db:"id,pk"`
	Email     string    `db:"email"`
	Name      string    `db:"name"`
	Age       *int      `db:"age"`
	CreatedAt time.Time `db:"created_at,created_at"`
}

func (schemaDiffUser) TableName() string { return "users" }

type zdtUser struct {
	ID   int64  `db:"id,pk"`
	Nick string `db:"nick"`
}

func (zdtUser) TableName() string { return "zdt_users" }

func TestSchemaDiffAddColumnFromModels(t *testing.T) {
	withFreshRegistry(t)
	db := setupSQLiteDB(t)

	snap, err := BuildSchemaSnapshotFromModels(schemaDiffUser{})
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	diff, err := DiffLiveSchemaFromSnapshot(context.Background(), db, snap, SchemaDiffOptions{Dialect: DialectSQLite})
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	found := false
	for _, ch := range diff.Changes {
		if ch.Kind == ChangeAddColumn && ch.Table == "users" && ch.Column == "age" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected add age column, diff=%+v", diff.Changes)
	}
}

func TestSchemaDiffDestructiveDropColumn(t *testing.T) {
	withFreshRegistry(t)
	db := setupSQLiteDB(t)

	snap, err := BuildSchemaSnapshotFromModels(schemaDiffUser{})
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	diff, err := DiffLiveSchemaFromSnapshot(context.Background(), db, snap, SchemaDiffOptions{Dialect: DialectSQLite, IncludeDestructive: true})
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	foundDrop := false
	for _, ch := range diff.Changes {
		if ch.Kind == ChangeDropColumn && ch.Table == "users" && ch.Column == "password" {
			foundDrop = true
			break
		}
	}
	if !foundDrop {
		t.Fatalf("expected drop password column in destructive mode")
	}
}

func TestWriteDiffMigrationFiles(t *testing.T) {
	withFreshRegistry(t)
	db := setupSQLiteDB(t)
	snap, err := BuildSchemaSnapshotFromModels(schemaDiffUser{})
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	diff, err := DiffLiveSchemaFromSnapshot(context.Background(), db, snap, SchemaDiffOptions{Dialect: DialectSQLite})
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	dir := t.TempDir()
	if err := WriteDiffMigrationFiles(dir, 100, "sync_users", diff); err != nil {
		t.Fatalf("write diff migrations: %v", err)
	}
	up, err := os.ReadFile(filepath.Join(dir, "100_sync_users.up.sql"))
	if err != nil {
		t.Fatalf("read up: %v", err)
	}
	if !strings.Contains(string(up), `ADD COLUMN "age"`) {
		t.Fatalf("unexpected up sql: %s", string(up))
	}
}

func TestZeroDowntimePlanClassification(t *testing.T) {
	withFreshRegistry(t)
	db := setupSQLiteDB(t)
	_, err := db.NewQuery(`CREATE TABLE zdt_users (id INTEGER PRIMARY KEY, nick TEXT NULL);`).Execute()
	if err != nil {
		t.Fatalf("create zdt table: %v", err)
	}
	snap, err := BuildSchemaSnapshotFromModels(zdtUser{})
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	diff, err := DiffLiveSchemaFromSnapshot(context.Background(), db, snap, SchemaDiffOptions{Dialect: DialectSQLite})
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	plan := BuildZeroDowntimePlan(diff)
	if len(plan.Contract) == 0 {
		t.Fatalf("expected contract phase changes")
	}
	if len(plan.Backfill) == 0 {
		t.Fatalf("expected backfill hints")
	}
}

func TestWriteZeroDowntimeMigrationFiles(t *testing.T) {
	plan := &ZeroDowntimePlan{
		Dialect:  DialectSQLite,
		Expand:   []SchemaChange{{Kind: ChangeAddColumn, Table: "users", Column: "x", UpSQL: `ALTER TABLE "users" ADD COLUMN "x" TEXT`, DownSQL: `ALTER TABLE "users" DROP COLUMN "x"`}},
		Backfill: []string{"-- backfill users.x"},
		Contract: []SchemaChange{{Kind: ChangeDropColumn, Table: "users", Column: "y", UpSQL: `ALTER TABLE "users" DROP COLUMN "y"`, DownSQL: `ALTER TABLE "users" ADD COLUMN "y" TEXT`}},
	}
	dir := t.TempDir()
	if err := WriteZeroDowntimeMigrationFiles(dir, 200, "zdt_users", plan); err != nil {
		t.Fatalf("write zdt files: %v", err)
	}
	paths := []string{
		filepath.Join(dir, "200_zdt_users_expand.up.sql"),
		filepath.Join(dir, "201_zdt_users_backfill.up.sql"),
		filepath.Join(dir, "202_zdt_users_contract.up.sql"),
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected file %s: %v", p, err)
		}
	}
}
