package orm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pafthang/dbx"
)

var migrationMu sync.Mutex

// Migration is an ordered DB migration unit.
type Migration struct {
	Version int64
	Name    string
	UpSQL   string
	DownSQL string
}

// MigrationRunner executes SQL migrations over orm.DB.
type MigrationRunner struct {
	DB              DB
	TableName       string
	LockName        string
	LockTTL         time.Duration
	UseDBLock       bool
	UseProcessMutex bool
}

// MigrationStatus contains migration state snapshot.
type MigrationStatus struct {
	CurrentVersion int64
	Applied        []int64
	Pending        []Migration
	Dirty          bool
	LastError      string
	Locked         bool
	LockOwner      string
}

// MigrationPlan describes directional migration actions from current state.
type MigrationPlan struct {
	CurrentVersion int64
	TargetVersion  int64
	Up             []Migration
	Down           []Migration
	Dirty          bool
	LastError      string
}

// NewMigrationRunner creates a new migration runner.
func NewMigrationRunner(db DB) *MigrationRunner {
	return &MigrationRunner{
		DB:              db,
		TableName:       "schema_migrations",
		LockName:        "global",
		LockTTL:         5 * time.Minute,
		UseDBLock:       true,
		UseProcessMutex: true,
	}
}

// MigrateUp applies all pending migrations in ascending version order.
func (r *MigrationRunner) MigrateUp(ctx context.Context, migrations []Migration) error {
	if r == nil || r.DB == nil {
		return ErrInvalidQuery.with("migrate_up", "", "", fmt.Errorf("migration runner db is nil"))
	}
	return r.withMigrationSafety(ctx, "migrate_up", func(ctx context.Context) error {
		applied, err := r.appliedChecksums(ctx)
		if err != nil {
			return err
		}
		sort.Slice(migrations, func(i, j int) bool { return migrations[i].Version < migrations[j].Version })
		for _, m := range migrations {
			checksum := migrationChecksum(m)
			if got, ok := applied[m.Version]; ok {
				if got != checksum {
					return ErrConflict.with("migrate_up", "", m.Name, fmt.Errorf("checksum mismatch for applied migration version %d", m.Version))
				}
				continue
			}
			if m.UpSQL == "" {
				return ErrInvalidQuery.with("migrate_up", "", m.Name, fmt.Errorf("empty up sql"))
			}
			if err := r.setDirty(ctx, true, ""); err != nil {
				return err
			}
			q := r.DB.NewQuery(m.UpSQL)
			if ctx != nil {
				q.WithContext(ctx)
			}
			if _, err := q.Execute(); err != nil {
				_ = r.setDirty(ctx, true, err.Error())
				return wrapQueryError(ErrInvalidQuery, "migrate_up", "", m.Name, err)
			}
			iq := r.DB.Insert(r.TableName, map[string]any{
				"version":    m.Version,
				"name":       m.Name,
				"checksum":   checksum,
				"applied_at": time.Now().UTC(),
			})
			if ctx != nil {
				iq.WithContext(ctx)
			}
			if _, err := iq.Execute(); err != nil {
				_ = r.setDirty(ctx, true, err.Error())
				return wrapQueryError(ErrInvalidQuery, "migrate_up", "", m.Name, err)
			}
			if err := r.setDirty(ctx, false, ""); err != nil {
				return err
			}
		}
		return nil
	})
}

// MigrateDown rolls back latest `steps` applied migrations.
func (r *MigrationRunner) MigrateDown(ctx context.Context, migrations []Migration, steps int) error {
	if r == nil || r.DB == nil {
		return ErrInvalidQuery.with("migrate_down", "", "", fmt.Errorf("migration runner db is nil"))
	}
	if steps <= 0 {
		return nil
	}
	return r.withMigrationSafety(ctx, "migrate_down", func(ctx context.Context) error {
		byVersion := map[int64]Migration{}
		for _, m := range migrations {
			byVersion[m.Version] = m
		}
		appliedList, err := r.appliedList(ctx)
		if err != nil {
			return err
		}
		if len(appliedList) == 0 {
			return nil
		}
		for i := len(appliedList) - 1; i >= 0 && steps > 0; i-- {
			v := appliedList[i]
			m, ok := byVersion[v]
			if !ok {
				return ErrInvalidQuery.with("migrate_down", "", fmt.Sprintf("%d", v), fmt.Errorf("migration definition not found"))
			}
			if strings.TrimSpace(m.DownSQL) == "" {
				return ErrInvalidQuery.with("migrate_down", "", m.Name, fmt.Errorf("empty down sql"))
			}
			if err := r.setDirty(ctx, true, ""); err != nil {
				return err
			}
			q := r.DB.NewQuery(m.DownSQL)
			if ctx != nil {
				q.WithContext(ctx)
			}
			if _, err := q.Execute(); err != nil {
				_ = r.setDirty(ctx, true, err.Error())
				return wrapQueryError(ErrInvalidQuery, "migrate_down", "", m.Name, err)
			}
			dq := r.DB.Delete(r.TableName, dbx.HashExp{"version": v})
			if ctx != nil {
				dq.WithContext(ctx)
			}
			if _, err := dq.Execute(); err != nil {
				_ = r.setDirty(ctx, true, err.Error())
				return wrapQueryError(ErrInvalidQuery, "migrate_down", "", m.Name, err)
			}
			if err := r.setDirty(ctx, false, ""); err != nil {
				return err
			}
			steps--
		}
		return nil
	})
}

// Status returns current migration status for provided migration set.
func (r *MigrationRunner) Status(ctx context.Context, migrations []Migration) (*MigrationStatus, error) {
	if r == nil || r.DB == nil {
		return nil, ErrInvalidQuery.with("migration_status", "", "", fmt.Errorf("migration runner db is nil"))
	}
	if r.UseProcessMutex {
		migrationMu.Lock()
		defer migrationMu.Unlock()
	}
	if err := r.ensureTable(ctx); err != nil {
		return nil, err
	}
	applied, err := r.appliedList(ctx)
	if err != nil {
		return nil, err
	}
	appliedSet := map[int64]struct{}{}
	for _, v := range applied {
		appliedSet[v] = struct{}{}
	}
	pending := make([]Migration, 0)
	for _, m := range migrations {
		if _, ok := appliedSet[m.Version]; !ok {
			pending = append(pending, m)
		}
	}
	sort.Slice(pending, func(i, j int) bool { return pending[i].Version < pending[j].Version })
	current := int64(0)
	for _, v := range applied {
		if v > current {
			current = v
		}
	}
	st, err := r.readState(ctx)
	if err != nil {
		return nil, err
	}
	lk, err := r.readLock(ctx)
	if err != nil {
		return nil, err
	}
	return &MigrationStatus{
		CurrentVersion: current,
		Applied:        applied,
		Pending:        pending,
		Dirty:          st.Dirty,
		LastError:      st.LastError,
		Locked:         lk.Locked,
		LockOwner:      lk.Owner,
	}, nil
}

// Validate checks migration definition consistency.
func (r *MigrationRunner) Validate(migrations []Migration) error {
	if len(migrations) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(migrations))
	for _, m := range migrations {
		if m.Version <= 0 {
			return ErrInvalidQuery.with("migration_validate", "", m.Name, fmt.Errorf("version must be positive"))
		}
		if strings.TrimSpace(m.Name) == "" {
			return ErrInvalidQuery.with("migration_validate", "", "", fmt.Errorf("migration name is empty for version %d", m.Version))
		}
		if strings.TrimSpace(m.UpSQL) == "" {
			return ErrInvalidQuery.with("migration_validate", "", m.Name, fmt.Errorf("empty up sql for version %d", m.Version))
		}
		if _, ok := seen[m.Version]; ok {
			return ErrConflict.with("migration_validate", "", m.Name, fmt.Errorf("duplicate migration version %d", m.Version))
		}
		seen[m.Version] = struct{}{}
	}
	return nil
}

// Plan computes migration steps required to reach targetVersion.
func (r *MigrationRunner) Plan(ctx context.Context, migrations []Migration, targetVersion int64) (*MigrationPlan, error) {
	if r == nil || r.DB == nil {
		return nil, ErrInvalidQuery.with("migration_plan", "", "", fmt.Errorf("migration runner db is nil"))
	}
	if err := r.Validate(migrations); err != nil {
		return nil, err
	}
	st, err := r.Status(ctx, migrations)
	if err != nil {
		return nil, err
	}
	plan := &MigrationPlan{
		CurrentVersion: st.CurrentVersion,
		TargetVersion:  targetVersion,
		Dirty:          st.Dirty,
		LastError:      st.LastError,
		Up:             []Migration{},
		Down:           []Migration{},
	}
	if targetVersion == st.CurrentVersion {
		return plan, nil
	}
	byVersion := map[int64]Migration{}
	for _, m := range migrations {
		byVersion[m.Version] = m
	}
	if targetVersion > 0 {
		if _, ok := byVersion[targetVersion]; !ok {
			return nil, ErrInvalidQuery.with("migration_plan", "", "", fmt.Errorf("target version %d not found in migrations", targetVersion))
		}
	}
	if targetVersion > st.CurrentVersion {
		sort.Slice(migrations, func(i, j int) bool { return migrations[i].Version < migrations[j].Version })
		for _, m := range migrations {
			if m.Version > st.CurrentVersion && m.Version <= targetVersion {
				plan.Up = append(plan.Up, m)
			}
		}
		return plan, nil
	}
	for i := len(st.Applied) - 1; i >= 0; i-- {
		v := st.Applied[i]
		if v <= targetVersion {
			break
		}
		m, ok := byVersion[v]
		if !ok {
			return nil, ErrInvalidQuery.with("migration_plan", "", fmt.Sprintf("%d", v), fmt.Errorf("migration definition not found for applied version"))
		}
		if strings.TrimSpace(m.DownSQL) == "" {
			return nil, ErrInvalidQuery.with("migration_plan", "", m.Name, fmt.Errorf("empty down sql for version %d", m.Version))
		}
		plan.Down = append(plan.Down, m)
	}
	return plan, nil
}

// MigrateTo moves schema state to targetVersion by applying up/down migrations.
func (r *MigrationRunner) MigrateTo(ctx context.Context, migrations []Migration, targetVersion int64) error {
	plan, err := r.Plan(ctx, migrations, targetVersion)
	if err != nil {
		return err
	}
	if plan.Dirty {
		return ErrConflict.with("migrate_to", "", "", fmt.Errorf("migration state is dirty: %s", plan.LastError))
	}
	if len(plan.Up) > 0 {
		return r.MigrateUp(ctx, plan.Up)
	}
	if len(plan.Down) > 0 {
		return r.MigrateDown(ctx, migrations, len(plan.Down))
	}
	return nil
}

// ForceVersion rewrites migration history table to match targetVersion without executing SQL.
func (r *MigrationRunner) ForceVersion(ctx context.Context, migrations []Migration, targetVersion int64) error {
	if r == nil || r.DB == nil {
		return ErrInvalidQuery.with("migration_force", "", "", fmt.Errorf("migration runner db is nil"))
	}
	if targetVersion < 0 {
		return ErrInvalidQuery.with("migration_force", "", "", fmt.Errorf("target version must be >= 0"))
	}
	if err := r.Validate(migrations); err != nil {
		return err
	}
	byVersion := map[int64]Migration{}
	for _, m := range migrations {
		byVersion[m.Version] = m
	}
	if targetVersion > 0 {
		if _, ok := byVersion[targetVersion]; !ok {
			return ErrInvalidQuery.with("migration_force", "", "", fmt.Errorf("target version %d not found in migrations", targetVersion))
		}
	}
	if r.UseProcessMutex {
		migrationMu.Lock()
		defer migrationMu.Unlock()
	}
	if err := r.ensureTable(ctx); err != nil {
		return err
	}
	owner := ""
	if r.UseDBLock {
		var err error
		owner, err = r.acquireDBLock(ctx)
		if err != nil {
			return ErrConflict.with("migration_force", "", "", err)
		}
		defer func() {
			_ = r.releaseDBLock(ctx, owner)
		}()
	}
	dq := r.DB.NewQuery(fmt.Sprintf("DELETE FROM %s WHERE version > {:target}", r.TableName)).Bind(dbx.Params{
		"target": targetVersion,
	})
	if ctx != nil {
		dq.WithContext(ctx)
	}
	if _, err := dq.Execute(); err != nil {
		return wrapQueryError(ErrInvalidQuery, "migration_force", "", "", err)
	}
	if targetVersion > 0 {
		ordered := make([]Migration, 0, len(migrations))
		for _, m := range migrations {
			if m.Version <= targetVersion {
				ordered = append(ordered, m)
			}
		}
		sort.Slice(ordered, func(i, j int) bool { return ordered[i].Version < ordered[j].Version })
		for _, m := range ordered {
			iq := r.DB.UpsertOnConflict(r.TableName, dbx.Params{
				"version":    m.Version,
				"name":       m.Name,
				"checksum":   migrationChecksum(m),
				"applied_at": time.Now().UTC(),
			}, dbx.Conflict("version").DoUpdateSet(
				dbx.Params{
					"name":       m.Name,
					"checksum":   migrationChecksum(m),
					"applied_at": time.Now().UTC(),
				},
			))
			if ctx != nil {
				iq.WithContext(ctx)
			}
			if _, err := iq.Execute(); err != nil {
				return wrapQueryError(ErrInvalidQuery, "migration_force", "", m.Name, err)
			}
		}
	}
	return r.setDirty(ctx, false, "")
}

// ClearDirty resets migration dirty flag and last error manually.
func (r *MigrationRunner) ClearDirty(ctx context.Context) error {
	if r == nil || r.DB == nil {
		return ErrInvalidQuery.with("migration_clear_dirty", "", "", fmt.Errorf("migration runner db is nil"))
	}
	if r.UseProcessMutex {
		migrationMu.Lock()
		defer migrationMu.Unlock()
	}
	if err := r.ensureTable(ctx); err != nil {
		return err
	}
	return r.setDirty(ctx, false, "")
}

// LoadMigrationsDir loads migrations from *.up.sql/*.down.sql files.
// Filename format: <version>_<name>.up.sql and <version>_<name>.down.sql
func LoadMigrationsDir(dir string) ([]Migration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	re := regexp.MustCompile(`^([0-9]+)_([a-zA-Z0-9_\-]+)\.(up|down)\.sql$`)
	byVersion := map[int64]*Migration{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		matches := re.FindStringSubmatch(e.Name())
		if len(matches) != 4 {
			continue
		}
		ver, err := strconv.ParseInt(matches[1], 10, 64)
		if err != nil {
			return nil, err
		}
		name := matches[2]
		side := matches[3]
		path := filepath.Join(dir, e.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		m := byVersion[ver]
		if m == nil {
			m = &Migration{Version: ver, Name: name}
			byVersion[ver] = m
		}
		if m.Name == "" {
			m.Name = name
		}
		switch side {
		case "up":
			m.UpSQL = string(b)
		case "down":
			m.DownSQL = string(b)
		}
	}
	out := make([]Migration, 0, len(byVersion))
	for _, m := range byVersion {
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

func (r *MigrationRunner) ensureTable(ctx context.Context) error {
	sql := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (version BIGINT PRIMARY KEY, name TEXT NOT NULL, checksum TEXT NOT NULL DEFAULT '', applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP);`, r.TableName)
	q := r.DB.NewQuery(sql)
	if ctx != nil {
		q.WithContext(ctx)
	}
	if _, err := q.Execute(); err != nil {
		return err
	}
	lockSQL := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (lock_name TEXT PRIMARY KEY, locked BOOLEAN NOT NULL DEFAULT FALSE, lock_owner TEXT NOT NULL DEFAULT '', locked_at TIMESTAMP NULL);`, r.lockTableName())
	lq := r.DB.NewQuery(lockSQL)
	if ctx != nil {
		lq.WithContext(ctx)
	}
	if _, err := lq.Execute(); err != nil {
		return err
	}
	stateSQL := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (id INTEGER PRIMARY KEY, dirty BOOLEAN NOT NULL DEFAULT FALSE, last_error TEXT NOT NULL DEFAULT '', updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP);`, r.stateTableName())
	sq := r.DB.NewQuery(stateSQL)
	if ctx != nil {
		sq.WithContext(ctx)
	}
	if _, err := sq.Execute(); err != nil {
		return err
	}
	return r.ensureStateRows(ctx)
}

func (r *MigrationRunner) appliedChecksums(ctx context.Context) (map[int64]string, error) {
	q := r.DB.Select("version", "checksum").From(r.TableName)
	if ctx != nil {
		q.WithContext(ctx)
	}
	rows, err := q.Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]string{}
	for rows.Next() {
		var v int64
		var c string
		if err := rows.Scan(&v, &c); err != nil {
			return nil, err
		}
		out[v] = c
	}
	return out, rows.Err()
}

func (r *MigrationRunner) appliedList(ctx context.Context) ([]int64, error) {
	q := r.DB.Select("version").From(r.TableName).OrderBy("version ASC")
	if ctx != nil {
		q.WithContext(ctx)
	}
	rows, err := q.Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]int64, 0)
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func migrationChecksum(m Migration) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(m.UpSQL) + "|" + strings.TrimSpace(m.DownSQL)))
	return hex.EncodeToString(sum[:])
}

type migrationStateRow struct {
	Dirty     bool
	LastError string
}

type migrationLockRow struct {
	Locked bool
	Owner  string
}

func (r *MigrationRunner) withMigrationSafety(ctx context.Context, op string, fn func(context.Context) error) error {
	if r.UseProcessMutex {
		migrationMu.Lock()
		defer migrationMu.Unlock()
	}
	if err := r.ensureTable(ctx); err != nil {
		return err
	}
	lockOwner := ""
	if r.UseDBLock {
		var err error
		lockOwner, err = r.acquireDBLock(ctx)
		if err != nil {
			return ErrConflict.with(op, "", "", err)
		}
		defer func() {
			_ = r.releaseDBLock(ctx, lockOwner)
		}()
	}
	st, err := r.readState(ctx)
	if err != nil {
		return err
	}
	if st.Dirty {
		return ErrConflict.with(op, "", "", fmt.Errorf("migration state is dirty: %s", st.LastError))
	}
	return fn(ctx)
}

func (r *MigrationRunner) lockTableName() string  { return r.TableName + "_lock" }
func (r *MigrationRunner) stateTableName() string { return r.TableName + "_state" }

func (r *MigrationRunner) ensureStateRows(ctx context.Context) error {
	now := time.Now().UTC()
	lockUpsert := r.DB.UpsertOnConflict(r.lockTableName(), dbx.Params{
		"lock_name":  r.LockName,
		"locked":     false,
		"lock_owner": "",
		"locked_at":  now,
	}, dbx.Conflict("lock_name").DoNothing())
	if ctx != nil {
		lockUpsert.WithContext(ctx)
	}
	if _, err := lockUpsert.Execute(); err != nil {
		return err
	}
	stateUpsert := r.DB.UpsertOnConflict(r.stateTableName(), dbx.Params{
		"id":         1,
		"dirty":      false,
		"last_error": "",
		"updated_at": now,
	}, dbx.Conflict("id").DoNothing())
	if ctx != nil {
		stateUpsert.WithContext(ctx)
	}
	_, err := stateUpsert.Execute()
	return err
}

func (r *MigrationRunner) acquireDBLock(ctx context.Context) (string, error) {
	owner := fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UTC().UnixNano())
	now := time.Now().UTC()
	q := r.DB.Update(r.lockTableName(), dbx.Params{
		"locked":     true,
		"lock_owner": owner,
		"locked_at":  now,
	}, dbx.HashExp{
		"lock_name": r.LockName,
		"locked":    false,
	})
	if ctx != nil {
		q.WithContext(ctx)
	}
	res, err := q.Execute()
	if err != nil {
		return "", err
	}
	if rows, _ := res.RowsAffected(); rows > 0 {
		return owner, nil
	}
	if r.LockTTL > 0 {
		expire := now.Add(-r.LockTTL)
		eq := r.DB.NewQuery(
			fmt.Sprintf("UPDATE %s SET locked = {:locked}, lock_owner = {:owner}, locked_at = {:now} WHERE lock_name = {:name} AND locked = TRUE AND locked_at < {:exp}", r.lockTableName()),
		).Bind(dbx.Params{
			"locked": true,
			"owner":  owner,
			"now":    now,
			"name":   r.LockName,
			"exp":    expire,
		})
		if ctx != nil {
			eq.WithContext(ctx)
		}
		r2, err := eq.Execute()
		if err != nil {
			return "", err
		}
		if rows, _ := r2.RowsAffected(); rows > 0 {
			return owner, nil
		}
	}
	return "", fmt.Errorf("db migration lock is held")
}

func (r *MigrationRunner) releaseDBLock(ctx context.Context, owner string) error {
	if owner == "" {
		return nil
	}
	q := r.DB.Update(r.lockTableName(), dbx.Params{
		"locked":     false,
		"lock_owner": "",
		"locked_at":  time.Now().UTC(),
	}, dbx.HashExp{
		"lock_name":  r.LockName,
		"lock_owner": owner,
	})
	if ctx != nil {
		q.WithContext(ctx)
	}
	_, err := q.Execute()
	return err
}

func (r *MigrationRunner) setDirty(ctx context.Context, dirty bool, lastErr string) error {
	q := r.DB.Update(r.stateTableName(), dbx.Params{
		"dirty":      dirty,
		"last_error": lastErr,
		"updated_at": time.Now().UTC(),
	}, dbx.HashExp{"id": 1})
	if ctx != nil {
		q.WithContext(ctx)
	}
	_, err := q.Execute()
	return err
}

func (r *MigrationRunner) readState(ctx context.Context) (migrationStateRow, error) {
	row := migrationStateRow{}
	q := r.DB.Select("dirty", "last_error").From(r.stateTableName()).Where(dbx.HashExp{"id": 1}).Limit(1)
	if ctx != nil {
		q.WithContext(ctx)
	}
	rows, err := q.Rows()
	if err != nil {
		return row, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return row, err
		}
		return row, fmt.Errorf("migration state row not found")
	}
	var dirtyRaw any
	var lastErr string
	if err := rows.Scan(&dirtyRaw, &lastErr); err != nil {
		return row, err
	}
	b, err := asBool(dirtyRaw)
	if err != nil {
		return row, err
	}
	row.Dirty = b
	row.LastError = lastErr
	return row, nil
}

func (r *MigrationRunner) readLock(ctx context.Context) (migrationLockRow, error) {
	row := migrationLockRow{}
	q := r.DB.Select("locked", "lock_owner").From(r.lockTableName()).Where(dbx.HashExp{"lock_name": r.LockName}).Limit(1)
	if ctx != nil {
		q.WithContext(ctx)
	}
	rows, err := q.Rows()
	if err != nil {
		return row, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return row, err
		}
		return row, fmt.Errorf("migration lock row not found")
	}
	var lockedRaw any
	var owner string
	if err := rows.Scan(&lockedRaw, &owner); err != nil {
		return row, err
	}
	b, err := asBool(lockedRaw)
	if err != nil {
		return row, err
	}
	row.Locked = b
	row.Owner = owner
	return row, nil
}

func asBool(v any) (bool, error) {
	switch x := v.(type) {
	case bool:
		return x, nil
	case int64:
		return x != 0, nil
	case int32:
		return x != 0, nil
	case int:
		return x != 0, nil
	case uint64:
		return x != 0, nil
	case uint:
		return x != 0, nil
	case []byte:
		return asBool(string(x))
	case string:
		s := strings.TrimSpace(strings.ToLower(x))
		switch s {
		case "1", "t", "true", "y", "yes":
			return true, nil
		case "0", "f", "false", "n", "no", "":
			return false, nil
		default:
			return false, fmt.Errorf("cannot parse bool value %q", x)
		}
	default:
		return false, fmt.Errorf("unsupported bool type: %T", v)
	}
}
