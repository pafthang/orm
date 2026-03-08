package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pafthang/dbx"
	"github.com/pafthang/orm"
	_ "modernc.org/sqlite"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "migrate":
		if err := runMigrate(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "migrate:", err)
			os.Exit(1)
		}
	case "codegen":
		if err := runCodegen(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "codegen:", err)
			os.Exit(1)
		}
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Println("ormtool migrate|codegen ...")
}

func runMigrate(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing migrate action: up|down|status|plan|validate|goto|force|clear-dirty|diff|zdt")
	}
	action := args[0]
	fs := flag.NewFlagSet("migrate "+action, flag.ContinueOnError)
	driver := fs.String("driver", "sqlite", "database driver: sqlite|pgx")
	dsn := fs.String("dsn", "", "database dsn")
	dir := fs.String("dir", "", "migrations directory")
	steps := fs.Int("steps", 1, "down steps")
	target := fs.Int64("target", 0, "target migration version (for plan/goto/force)")
	dialect := fs.String("dialect", "auto", "sql dialect: auto|sqlite|postgres")
	snapshotPath := fs.String("snapshot", "", "desired schema snapshot JSON path")
	version := fs.Int64("version", 0, "migration version for generated files")
	name := fs.String("name", "auto_schema", "migration name for generated files")
	unsafeDrop := fs.Bool("unsafe-drop", false, "include destructive changes (drop column/table)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if actionNeedsDB(action) && *dsn == "" {
		return fmt.Errorf("--dsn is required")
	}
	if actionNeedsDir(action) && *dir == "" {
		return fmt.Errorf("--dir is required")
	}

	migrations := []orm.Migration{}
	var err error
	if *dir != "" && actionUsesMigrationSet(action) {
		migrations, err = orm.LoadMigrationsDir(*dir)
		if err != nil {
			return err
		}
	}

	if action == "diff" || action == "zdt" {
		if *snapshotPath == "" {
			return fmt.Errorf("--snapshot is required")
		}
		if *version <= 0 {
			return fmt.Errorf("--version is required and must be > 0")
		}
	}

	var db *dbx.DB
	if actionNeedsDB(action) {
		db, err = dbx.Open(*driver, *dsn)
		if err != nil {
			return err
		}
		defer db.Close()
	}

	r := orm.NewMigrationRunner(db)
	ctx := context.Background()
	switch action {
	case "up":
		return r.MigrateUp(ctx, migrations)
	case "down":
		return r.MigrateDown(ctx, migrations, *steps)
	case "status":
		st, err := r.Status(ctx, migrations)
		if err != nil {
			return err
		}
		fmt.Printf("current_version=%d applied=%d pending=%d dirty=%t locked=%t\n", st.CurrentVersion, len(st.Applied), len(st.Pending), st.Dirty, st.Locked)
		return nil
	case "validate":
		return r.Validate(migrations)
	case "plan":
		plan, err := r.Plan(ctx, migrations, *target)
		if err != nil {
			return err
		}
		fmt.Printf("current=%d target=%d up=%d down=%d dirty=%t\n", plan.CurrentVersion, plan.TargetVersion, len(plan.Up), len(plan.Down), plan.Dirty)
		return nil
	case "goto":
		return r.MigrateTo(ctx, migrations, *target)
	case "force":
		return r.ForceVersion(ctx, migrations, *target)
	case "clear-dirty":
		return r.ClearDirty(ctx)
	case "diff":
		snap, err := orm.LoadSchemaSnapshot(*snapshotPath)
		if err != nil {
			return err
		}
		diff, err := orm.DiffLiveSchemaFromSnapshot(ctx, db, snap, orm.SchemaDiffOptions{Dialect: orm.SQLDialect(*dialect), IncludeDestructive: *unsafeDrop})
		if err != nil {
			return err
		}
		if err := orm.WriteDiffMigrationFiles(*dir, *version, *name, diff); err != nil {
			return err
		}
		fmt.Printf("diff changes=%d warnings=%d\n", len(diff.Changes), len(diff.Warnings))
		return nil
	case "zdt":
		snap, err := orm.LoadSchemaSnapshot(*snapshotPath)
		if err != nil {
			return err
		}
		diff, err := orm.DiffLiveSchemaFromSnapshot(ctx, db, snap, orm.SchemaDiffOptions{Dialect: orm.SQLDialect(*dialect), IncludeDestructive: *unsafeDrop})
		if err != nil {
			return err
		}
		plan := orm.BuildZeroDowntimePlan(diff)
		if err := orm.WriteZeroDowntimeMigrationFiles(*dir, *version, *name, plan); err != nil {
			return err
		}
		fmt.Printf("zdt expand=%d backfill=%d contract=%d warnings=%d\n", len(plan.Expand), len(plan.Backfill), len(plan.Contract), len(plan.Warnings))
		return nil
	default:
		return fmt.Errorf("unknown migrate action: %s", action)
	}
}

func actionNeedsDB(action string) bool {
	switch action {
	case "up", "down", "status", "plan", "goto", "force", "clear-dirty", "diff", "zdt":
		return true
	default:
		return false
	}
}

func actionNeedsDir(action string) bool {
	switch action {
	case "up", "down", "validate", "plan", "goto", "force", "diff", "zdt":
		return true
	default:
		return false
	}
}

func actionUsesMigrationSet(action string) bool {
	switch action {
	case "up", "down", "validate", "plan", "goto", "force", "status":
		return true
	default:
		return false
	}
}

func runCodegen(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing codegen action: columns|repo|run")
	}
	action := args[0]
	fs := flag.NewFlagSet("codegen "+action, flag.ContinueOnError)
	name := fs.String("name", "", "model name")
	output := fs.String("output", "", "output file")
	columns := fs.String("columns", "", "comma-separated columns (for columns action)")
	pkType := fs.String("pk-type", "int64", "pk type (for repo action)")
	config := fs.String("config", "", "codegen config file (for run action)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	if action == "run" {
		if *config == "" {
			return fmt.Errorf("--config is required")
		}
		return orm.RunCodegenPipeline(*config)
	}

	if *name == "" {
		return fmt.Errorf("--name is required")
	}
	if *output == "" {
		return fmt.Errorf("--output is required")
	}

	var src string
	var err error
	switch action {
	case "columns":
		src, err = orm.GenerateColumnsConstFromSpec(*name, strings.Split(*columns, ","))
	case "repo":
		src, err = orm.GenerateRepositoryStubFromSpec(*name, *pkType)
	default:
		return fmt.Errorf("unknown codegen action: %s", action)
	}
	if err != nil {
		return err
	}
	return os.WriteFile(*output, []byte(src), 0o644)
}
