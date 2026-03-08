package orm

import "github.com/pafthang/dbx"

// DB is the dbx-based contract accepted by ORM operations.
type DB interface {
	NewQuery(sql string) *dbx.Query
	Select(cols ...string) *dbx.SelectQuery
	Insert(table string, cols dbx.Params) *dbx.Query
	InsertMany(table string, rows []dbx.Params) *dbx.Query
	InsertReturning(table string, cols dbx.Params, returning ...string) *dbx.Query
	Update(table string, cols dbx.Params, where dbx.Expression) *dbx.Query
	Delete(table string, where dbx.Expression) *dbx.Query
	Upsert(table string, cols dbx.Params, constraints ...string) *dbx.Query
	UpsertOnConflict(table string, cols dbx.Params, conflict dbx.OnConflict) *dbx.Query
}
