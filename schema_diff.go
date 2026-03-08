package orm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/pafthang/dbx"
)

type SQLDialect string

const (
	DialectAuto     SQLDialect = "auto"
	DialectSQLite   SQLDialect = "sqlite"
	DialectPostgres SQLDialect = "postgres"
)

type ColumnSchema struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Nullable   bool   `json:"nullable"`
	PrimaryKey bool   `json:"primary_key"`
}

type TableSchema struct {
	Name       string                   `json:"name"`
	Columns    map[string]*ColumnSchema `json:"columns"`
	PrimaryKey []string                 `json:"primary_key,omitempty"`
}

type SchemaSnapshot struct {
	Dialect SQLDialect              `json:"dialect"`
	Tables  map[string]*TableSchema `json:"tables"`
}

type SchemaDiffOptions struct {
	Dialect            SQLDialect
	IncludeDestructive bool
}

type SchemaChangeKind string

const (
	ChangeCreateTable SchemaChangeKind = "create_table"
	ChangeDropTable   SchemaChangeKind = "drop_table"
	ChangeAddColumn   SchemaChangeKind = "add_column"
	ChangeDropColumn  SchemaChangeKind = "drop_column"
	ChangeAlterType   SchemaChangeKind = "alter_type"
	ChangeSetNotNull  SchemaChangeKind = "set_not_null"
	ChangeDropNotNull SchemaChangeKind = "drop_not_null"
)

type SchemaChange struct {
	Kind       SchemaChangeKind `json:"kind"`
	Table      string           `json:"table"`
	Column     string           `json:"column,omitempty"`
	FromType   string           `json:"from_type,omitempty"`
	ToType     string           `json:"to_type,omitempty"`
	UpSQL      string           `json:"up_sql"`
	DownSQL    string           `json:"down_sql"`
	Breaking   bool             `json:"breaking"`
	Reversible bool             `json:"reversible"`
}

type SchemaDiff struct {
	Dialect  SQLDialect     `json:"dialect"`
	Changes  []SchemaChange `json:"changes"`
	Warnings []string       `json:"warnings,omitempty"`
}

type ZeroDowntimePlan struct {
	Dialect  SQLDialect     `json:"dialect"`
	Expand   []SchemaChange `json:"expand"`
	Backfill []string       `json:"backfill"`
	Contract []SchemaChange `json:"contract"`
	Warnings []string       `json:"warnings,omitempty"`
}

// BuildSchemaSnapshotFromModels builds canonical schema from model types.
func BuildSchemaSnapshotFromModels(models ...any) (*SchemaSnapshot, error) {
	if len(models) == 0 {
		return nil, ErrInvalidModel.with("schema_snapshot", "", "", fmt.Errorf("models are required"))
	}
	s := &SchemaSnapshot{Dialect: DialectAuto, Tables: map[string]*TableSchema{}}
	for _, m := range models {
		meta, err := DefaultRegistry.Resolve(m)
		if err != nil {
			return nil, err
		}
		ts := &TableSchema{Name: meta.Table, Columns: map[string]*ColumnSchema{}}
		for _, f := range meta.Fields {
			if f.IsIgnored || f.IsRelation {
				continue
			}
			typeName := canonicalTypeFromField(f)
			ts.Columns[f.DBName] = &ColumnSchema{
				Name:       f.DBName,
				Type:       typeName,
				Nullable:   f.IsNullable || isNullableType(f.Type),
				PrimaryKey: f.IsPK,
			}
		}
		sort.Slice(meta.PrimaryKeys, func(i, j int) bool { return meta.PrimaryKeys[i].DBName < meta.PrimaryKeys[j].DBName })
		for _, pk := range meta.PrimaryKeys {
			ts.PrimaryKey = append(ts.PrimaryKey, pk.DBName)
		}
		s.Tables[ts.Name] = ts
	}
	return s, nil
}

// SaveSchemaSnapshot writes schema snapshot to JSON file.
func SaveSchemaSnapshot(path string, snapshot *SchemaSnapshot) error {
	if snapshot == nil {
		return ErrInvalidQuery.with("schema_snapshot_save", "", "", fmt.Errorf("snapshot is nil"))
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// LoadSchemaSnapshot reads schema snapshot from JSON file.
func LoadSchemaSnapshot(path string) (*SchemaSnapshot, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s SchemaSnapshot
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, ErrInvalidQuery.with("schema_snapshot_load", "", "", err)
	}
	if s.Tables == nil {
		s.Tables = map[string]*TableSchema{}
	}
	for name, t := range s.Tables {
		if t == nil {
			return nil, ErrInvalidQuery.with("schema_snapshot_load", "", name, fmt.Errorf("table schema is nil"))
		}
		if t.Name == "" {
			t.Name = name
		}
		if t.Columns == nil {
			t.Columns = map[string]*ColumnSchema{}
		}
	}
	return &s, nil
}

// DiffLiveSchemaFromSnapshot compares live DB schema against desired snapshot.
func DiffLiveSchemaFromSnapshot(ctx context.Context, db DB, desired *SchemaSnapshot, opts SchemaDiffOptions) (*SchemaDiff, error) {
	if db == nil {
		return nil, ErrInvalidQuery.with("schema_diff", "", "", fmt.Errorf("db is nil"))
	}
	if desired == nil {
		return nil, ErrInvalidQuery.with("schema_diff", "", "", fmt.Errorf("desired snapshot is nil"))
	}
	dialect := opts.Dialect
	if dialect == "" || dialect == DialectAuto {
		var err error
		dialect, err = detectDialect(ctx, db)
		if err != nil {
			return nil, err
		}
	}
	live, err := introspectDBSchema(ctx, db, dialect)
	if err != nil {
		return nil, err
	}
	return diffSchemas(live, desired, dialect, opts), nil
}

// BuildZeroDowntimePlan classifies schema changes into expand/backfill/contract phases.
func BuildZeroDowntimePlan(diff *SchemaDiff) *ZeroDowntimePlan {
	if diff == nil {
		return &ZeroDowntimePlan{}
	}
	plan := &ZeroDowntimePlan{Dialect: diff.Dialect, Warnings: append([]string(nil), diff.Warnings...)}
	for _, ch := range diff.Changes {
		switch ch.Kind {
		case ChangeCreateTable, ChangeDropNotNull:
			plan.Expand = append(plan.Expand, ch)
		case ChangeAddColumn:
			if ch.Breaking {
				plan.Contract = append(plan.Contract, ch)
				plan.Backfill = append(plan.Backfill, fmt.Sprintf("-- backfill required for NOT NULL column: table=%s column=%s", ch.Table, ch.Column))
			} else {
				plan.Expand = append(plan.Expand, ch)
			}
		case ChangeSetNotNull, ChangeAlterType, ChangeDropColumn, ChangeDropTable:
			plan.Contract = append(plan.Contract, ch)
			if ch.Kind == ChangeSetNotNull {
				plan.Backfill = append(plan.Backfill, fmt.Sprintf("-- backfill required before NOT NULL: table=%s column=%s", ch.Table, ch.Column))
			}
			if ch.Kind == ChangeAlterType {
				plan.Backfill = append(plan.Backfill, fmt.Sprintf("-- verify cast safety before type change: table=%s column=%s from=%s to=%s", ch.Table, ch.Column, ch.FromType, ch.ToType))
			}
		}
	}
	if len(plan.Backfill) == 0 {
		plan.Backfill = []string{"-- no backfill steps generated"}
	}
	return plan
}

// WriteDiffMigrationFiles writes one migration pair from a schema diff.
func WriteDiffMigrationFiles(dir string, version int64, name string, diff *SchemaDiff) error {
	if diff == nil {
		return ErrInvalidQuery.with("schema_diff_write", "", "", fmt.Errorf("diff is nil"))
	}
	up := collectSQL(diff.Changes, false)
	down := collectSQL(reverseChanges(diff.Changes), true)
	return writeMigrationPair(dir, version, name, up, down)
}

// WriteZeroDowntimeMigrationFiles writes expand/backfill/contract migration triplet.
func WriteZeroDowntimeMigrationFiles(dir string, baseVersion int64, name string, plan *ZeroDowntimePlan) error {
	if plan == nil {
		return ErrInvalidQuery.with("zdt_write", "", "", fmt.Errorf("plan is nil"))
	}
	if err := writeMigrationPair(dir, baseVersion, name+"_expand", collectSQL(plan.Expand, false), collectSQL(reverseChanges(plan.Expand), true)); err != nil {
		return err
	}
	backfillUp := strings.Join(plan.Backfill, "\n") + "\n"
	backfillDown := "-- backfill phase rollback is manual\n"
	if err := writeMigrationPair(dir, baseVersion+1, name+"_backfill", backfillUp, backfillDown); err != nil {
		return err
	}
	return writeMigrationPair(dir, baseVersion+2, name+"_contract", collectSQL(plan.Contract, false), collectSQL(reverseChanges(plan.Contract), true))
}

func writeMigrationPair(dir string, version int64, name, upSQL, downSQL string) error {
	if version <= 0 {
		return ErrInvalidQuery.with("migration_write", "", "", fmt.Errorf("version must be positive"))
	}
	name = sanitizeMigrationName(name)
	if name == "" {
		name = "auto"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	upPath := filepath.Join(dir, fmt.Sprintf("%d_%s.up.sql", version, name))
	downPath := filepath.Join(dir, fmt.Sprintf("%d_%s.down.sql", version, name))
	if err := os.WriteFile(upPath, []byte(strings.TrimSpace(upSQL)+"\n"), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(downPath, []byte(strings.TrimSpace(downSQL)+"\n"), 0o644); err != nil {
		return err
	}
	return nil
}

func sanitizeMigrationName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, "-", "_")
	return name
}

func collectSQL(changes []SchemaChange, down bool) string {
	parts := make([]string, 0, len(changes)+1)
	for _, ch := range changes {
		sql := ch.UpSQL
		if down {
			sql = ch.DownSQL
		}
		sql = strings.TrimSpace(sql)
		if sql == "" {
			continue
		}
		if !strings.HasSuffix(sql, ";") {
			sql += ";"
		}
		parts = append(parts, sql)
	}
	if len(parts) == 0 {
		parts = append(parts, "-- no-op;")
	}
	return strings.Join(parts, "\n") + "\n"
}

func reverseChanges(in []SchemaChange) []SchemaChange {
	out := make([]SchemaChange, len(in))
	copy(out, in)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func diffSchemas(live, desired *SchemaSnapshot, dialect SQLDialect, opts SchemaDiffOptions) *SchemaDiff {
	d := &SchemaDiff{Dialect: dialect, Changes: []SchemaChange{}, Warnings: []string{}}
	if live == nil {
		live = &SchemaSnapshot{Dialect: dialect, Tables: map[string]*TableSchema{}}
	}
	if desired == nil {
		desired = &SchemaSnapshot{Dialect: dialect, Tables: map[string]*TableSchema{}}
	}
	tables := sortedKeys(desired.Tables)
	for _, table := range tables {
		dt := desired.Tables[table]
		lt, ok := live.Tables[table]
		if !ok {
			d.Changes = append(d.Changes, createTableChange(dialect, dt))
			continue
		}
		colNames := sortedKeys(dt.Columns)
		for _, c := range colNames {
			dc := dt.Columns[c]
			lc, ok := lt.Columns[c]
			if !ok {
				d.Changes = append(d.Changes, addColumnChange(dialect, table, dc))
				continue
			}
			if normalizeSQLType(dc.Type) != normalizeSQLType(lc.Type) {
				d.Changes = append(d.Changes, alterTypeChange(dialect, table, c, lc.Type, dc.Type))
			}
			if dc.Nullable != lc.Nullable {
				if dc.Nullable {
					d.Changes = append(d.Changes, dropNotNullChange(dialect, table, c))
				} else {
					d.Changes = append(d.Changes, setNotNullChange(dialect, table, c))
				}
			}
		}
		if opts.IncludeDestructive {
			for _, c := range sortedKeys(lt.Columns) {
				if _, ok := dt.Columns[c]; !ok {
					d.Changes = append(d.Changes, dropColumnChange(dialect, table, lt.Columns[c]))
				}
			}
		}
	}
	if opts.IncludeDestructive {
		for _, table := range sortedKeys(live.Tables) {
			if _, ok := desired.Tables[table]; !ok {
				d.Changes = append(d.Changes, dropTableChange(dialect, live.Tables[table]))
			}
		}
	}
	if dialect == DialectSQLite {
		for _, ch := range d.Changes {
			if ch.Kind == ChangeAlterType || ch.Kind == ChangeSetNotNull || ch.Kind == ChangeDropNotNull || ch.Kind == ChangeDropColumn {
				d.Warnings = append(d.Warnings, "sqlite may require table rebuild for some ALTER operations")
				break
			}
		}
	}
	return d
}

func createTableChange(dialect SQLDialect, table *TableSchema) SchemaChange {
	cols := make([]string, 0, len(table.Columns)+1)
	for _, name := range sortedKeys(table.Columns) {
		c := table.Columns[name]
		cols = append(cols, columnDefinition(dialect, c))
	}
	if len(table.PrimaryKey) > 1 {
		pkCols := make([]string, 0, len(table.PrimaryKey))
		for _, p := range table.PrimaryKey {
			pkCols = append(pkCols, quoteIdent(dialect, p))
		}
		cols = append(cols, fmt.Sprintf("PRIMARY KEY (%s)", strings.Join(pkCols, ", ")))
	}
	up := fmt.Sprintf("CREATE TABLE %s (%s)", quoteIdent(dialect, table.Name), strings.Join(cols, ", "))
	down := fmt.Sprintf("DROP TABLE %s", quoteIdent(dialect, table.Name))
	return SchemaChange{Kind: ChangeCreateTable, Table: table.Name, UpSQL: up, DownSQL: down, Reversible: true}
}

func dropTableChange(dialect SQLDialect, table *TableSchema) SchemaChange {
	up := fmt.Sprintf("DROP TABLE %s", quoteIdent(dialect, table.Name))
	down := createTableChange(dialect, table).UpSQL
	return SchemaChange{Kind: ChangeDropTable, Table: table.Name, UpSQL: up, DownSQL: down, Reversible: true, Breaking: true}
}

func addColumnChange(dialect SQLDialect, table string, c *ColumnSchema) SchemaChange {
	up := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", quoteIdent(dialect, table), columnDefinition(dialect, c))
	down := fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", quoteIdent(dialect, table), quoteIdent(dialect, c.Name))
	return SchemaChange{Kind: ChangeAddColumn, Table: table, Column: c.Name, ToType: c.Type, UpSQL: up, DownSQL: down, Reversible: true, Breaking: !c.Nullable}
}

func dropColumnChange(dialect SQLDialect, table string, c *ColumnSchema) SchemaChange {
	up := fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", quoteIdent(dialect, table), quoteIdent(dialect, c.Name))
	down := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", quoteIdent(dialect, table), columnDefinition(dialect, c))
	return SchemaChange{Kind: ChangeDropColumn, Table: table, Column: c.Name, FromType: c.Type, UpSQL: up, DownSQL: down, Reversible: true, Breaking: true}
}

func alterTypeChange(dialect SQLDialect, table, col, fromType, toType string) SchemaChange {
	up := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s", quoteIdent(dialect, table), quoteIdent(dialect, col), sqlTypeForDialect(dialect, toType))
	down := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s", quoteIdent(dialect, table), quoteIdent(dialect, col), sqlTypeForDialect(dialect, fromType))
	return SchemaChange{Kind: ChangeAlterType, Table: table, Column: col, FromType: fromType, ToType: toType, UpSQL: up, DownSQL: down, Reversible: true, Breaking: true}
}

func setNotNullChange(dialect SQLDialect, table, col string) SchemaChange {
	up := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET NOT NULL", quoteIdent(dialect, table), quoteIdent(dialect, col))
	down := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP NOT NULL", quoteIdent(dialect, table), quoteIdent(dialect, col))
	return SchemaChange{Kind: ChangeSetNotNull, Table: table, Column: col, UpSQL: up, DownSQL: down, Reversible: true, Breaking: true}
}

func dropNotNullChange(dialect SQLDialect, table, col string) SchemaChange {
	up := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP NOT NULL", quoteIdent(dialect, table), quoteIdent(dialect, col))
	down := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET NOT NULL", quoteIdent(dialect, table), quoteIdent(dialect, col))
	return SchemaChange{Kind: ChangeDropNotNull, Table: table, Column: col, UpSQL: up, DownSQL: down, Reversible: true}
}

func columnDefinition(dialect SQLDialect, c *ColumnSchema) string {
	parts := []string{quoteIdent(dialect, c.Name), sqlTypeForDialect(dialect, c.Type)}
	if !c.Nullable {
		parts = append(parts, "NOT NULL")
	}
	if c.PrimaryKey {
		parts = append(parts, "PRIMARY KEY")
	}
	return strings.Join(parts, " ")
}

func quoteIdent(_ SQLDialect, ident string) string {
	return `"` + strings.ReplaceAll(ident, `"`, `""`) + `"`
}

func sqlTypeForDialect(dialect SQLDialect, canonical string) string {
	canonical = normalizeSQLType(canonical)
	switch dialect {
	case DialectPostgres:
		switch canonical {
		case "integer":
			return "INTEGER"
		case "bigint":
			return "BIGINT"
		case "bool":
			return "BOOLEAN"
		case "float":
			return "REAL"
		case "double":
			return "DOUBLE PRECISION"
		case "timestamp":
			return "TIMESTAMPTZ"
		case "bytes":
			return "BYTEA"
		case "uuid":
			return "UUID"
		default:
			return "TEXT"
		}
	default:
		switch canonical {
		case "integer", "bigint":
			return "INTEGER"
		case "bool":
			return "BOOLEAN"
		case "float", "double":
			return "REAL"
		case "timestamp":
			return "TIMESTAMP"
		case "bytes":
			return "BLOB"
		default:
			return "TEXT"
		}
	}
}

func canonicalTypeFromField(f *FieldMeta) string {
	t := f.Type
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if strings.EqualFold(t.Name(), "UUID") {
		return "uuid"
	}
	if t.AssignableTo(reflect.TypeOf(time.Time{})) {
		return "timestamp"
	}
	switch t.Kind() {
	case reflect.Bool:
		return "bool"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32:
		return "integer"
	case reflect.Int64, reflect.Uint64:
		return "bigint"
	case reflect.Float32:
		return "float"
	case reflect.Float64:
		return "double"
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			return "bytes"
		}
		return "text"
	case reflect.String:
		return "text"
	default:
		return "text"
	}
}

func detectDialect(ctx context.Context, db DB) (SQLDialect, error) {
	q := db.NewQuery("SELECT sqlite_version()")
	if ctx != nil {
		q.WithContext(ctx)
	}
	if rows, err := q.Rows(); err == nil {
		rows.Close()
		return DialectSQLite, nil
	}
	q = db.NewQuery("SELECT current_database()")
	if ctx != nil {
		q.WithContext(ctx)
	}
	if rows, err := q.Rows(); err == nil {
		rows.Close()
		return DialectPostgres, nil
	}
	return "", ErrInvalidQuery.with("schema_detect_dialect", "", "", fmt.Errorf("unable to detect dialect"))
}

func introspectDBSchema(ctx context.Context, db DB, dialect SQLDialect) (*SchemaSnapshot, error) {
	s := &SchemaSnapshot{Dialect: dialect, Tables: map[string]*TableSchema{}}
	var err error
	switch dialect {
	case DialectSQLite:
		err = introspectSQLite(ctx, db, s)
	case DialectPostgres:
		err = introspectPostgres(ctx, db, s)
	default:
		err = ErrInvalidQuery.with("schema_introspect", "", "", fmt.Errorf("unsupported dialect %s", dialect))
	}
	if err != nil {
		return nil, err
	}
	return s, nil
}

func introspectSQLite(ctx context.Context, db DB, s *SchemaSnapshot) error {
	q := db.NewQuery("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'")
	if ctx != nil {
		q.WithContext(ctx)
	}
	rows, err := q.Rows()
	if err != nil {
		return err
	}
	defer rows.Close()
	tables := make([]string, 0)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, table := range tables {
		t := &TableSchema{Name: table, Columns: map[string]*ColumnSchema{}}
		pq := db.NewQuery(fmt.Sprintf("PRAGMA table_info(%s)", quoteIdent(DialectSQLite, table)))
		if ctx != nil {
			pq.WithContext(ctx)
		}
		pr, err := pq.Rows()
		if err != nil {
			return err
		}
		for pr.Next() {
			var cid int
			var name string
			var typ string
			var notNull int
			var def any
			var pk int
			if err := pr.Scan(&cid, &name, &typ, &notNull, &def, &pk); err != nil {
				pr.Close()
				return err
			}
			_ = cid
			_ = def
			c := &ColumnSchema{
				Name:       name,
				Type:       normalizeSQLType(typ),
				Nullable:   notNull == 0,
				PrimaryKey: pk > 0,
			}
			t.Columns[name] = c
			if c.PrimaryKey {
				t.PrimaryKey = append(t.PrimaryKey, name)
			}
		}
		if err := pr.Err(); err != nil {
			pr.Close()
			return err
		}
		pr.Close()
		s.Tables[table] = t
	}
	return nil
}

func introspectPostgres(ctx context.Context, db DB, s *SchemaSnapshot) error {
	q := db.NewQuery("SELECT table_name FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE'")
	if ctx != nil {
		q.WithContext(ctx)
	}
	rows, err := q.Rows()
	if err != nil {
		return err
	}
	defer rows.Close()
	tables := make([]string, 0)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, table := range tables {
		t := &TableSchema{Name: table, Columns: map[string]*ColumnSchema{}}
		cq := db.NewQuery("SELECT column_name, data_type, is_nullable FROM information_schema.columns WHERE table_schema='public' AND table_name={:table} ORDER BY ordinal_position").Bind(dbx.Params{"table": table})
		if ctx != nil {
			cq.WithContext(ctx)
		}
		cr, err := cq.Rows()
		if err != nil {
			return err
		}
		for cr.Next() {
			var col, typ, nullable string
			if err := cr.Scan(&col, &typ, &nullable); err != nil {
				cr.Close()
				return err
			}
			t.Columns[col] = &ColumnSchema{Name: col, Type: normalizeSQLType(typ), Nullable: strings.EqualFold(nullable, "YES")}
		}
		if err := cr.Err(); err != nil {
			cr.Close()
			return err
		}
		cr.Close()

		pkq := db.NewQuery(`SELECT kcu.column_name
FROM information_schema.table_constraints tc
JOIN information_schema.key_column_usage kcu
  ON tc.constraint_name = kcu.constraint_name
 AND tc.table_schema = kcu.table_schema
WHERE tc.constraint_type = 'PRIMARY KEY'
  AND tc.table_schema = 'public'
  AND tc.table_name = {:table}
ORDER BY kcu.ordinal_position`).Bind(dbx.Params{"table": table})
		if ctx != nil {
			pkq.WithContext(ctx)
		}
		pr, err := pkq.Rows()
		if err != nil {
			return err
		}
		for pr.Next() {
			var col string
			if err := pr.Scan(&col); err != nil {
				pr.Close()
				return err
			}
			t.PrimaryKey = append(t.PrimaryKey, col)
			if c, ok := t.Columns[col]; ok {
				c.PrimaryKey = true
			}
		}
		if err := pr.Err(); err != nil {
			pr.Close()
			return err
		}
		pr.Close()
		s.Tables[table] = t
	}
	return nil
}

func normalizeSQLType(t string) string {
	t = strings.TrimSpace(strings.ToLower(t))
	switch {
	case strings.Contains(t, "bigint"):
		return "bigint"
	case strings.Contains(t, "smallint"), strings.Contains(t, "int"):
		return "integer"
	case strings.Contains(t, "bool"):
		return "bool"
	case strings.Contains(t, "double"):
		return "double"
	case strings.Contains(t, "real"), strings.Contains(t, "float"), strings.Contains(t, "numeric"), strings.Contains(t, "decimal"):
		return "float"
	case strings.Contains(t, "timestamp"), strings.Contains(t, "date"), strings.Contains(t, "time"):
		return "timestamp"
	case strings.Contains(t, "bytea"), strings.Contains(t, "blob"), strings.Contains(t, "binary"):
		return "bytes"
	case strings.Contains(t, "uuid"):
		return "uuid"
	default:
		return "text"
	}
}

func sortedKeys[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
