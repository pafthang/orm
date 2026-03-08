package orm

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/pafthang/dbx"
	_ "modernc.org/sqlite"
)

type compareUser struct {
	ID        int64     `db:"id,pk"`
	Email     string    `db:"email"`
	Name      string    `db:"name"`
	CreatedAt time.Time `db:"created_at,created_at"`
	UpdatedAt time.Time `db:"updated_at,updated_at"`
}

func (compareUser) TableName() string { return "users" }

func setupCompareDB(b *testing.B) (*sql.DB, *dbx.DB, int64) {
	b.Helper()
	raw, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		b.Fatalf("open sql: %v", err)
	}
	b.Cleanup(func() { _ = raw.Close() })
	if _, err := raw.Exec(`
CREATE TABLE users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	email TEXT NOT NULL,
	name TEXT NOT NULL,
	created_at TIMESTAMP NOT NULL,
	updated_at TIMESTAMP NOT NULL
);`); err != nil {
		b.Fatalf("create schema raw: %v", err)
	}
	now := time.Now().UTC()
	res, err := raw.Exec(`INSERT INTO users (email, name, created_at, updated_at) VALUES (?, ?, ?, ?)`, "cmp@example.com", "cmp", now, now)
	if err != nil {
		b.Fatalf("seed raw: %v", err)
	}
	id, _ := res.LastInsertId()

	db, err := dbx.Open("sqlite", ":memory:")
	if err != nil {
		b.Fatalf("open dbx: %v", err)
	}
	b.Cleanup(func() { _ = db.Close() })
	if _, err := db.NewQuery(`
CREATE TABLE users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	email TEXT NOT NULL,
	name TEXT NOT NULL,
	created_at TIMESTAMP NOT NULL,
	updated_at TIMESTAMP NOT NULL
);`).Execute(); err != nil {
		b.Fatalf("create schema dbx: %v", err)
	}
	if _, err := db.Insert("users", dbx.Params{
		"email":      "cmp@example.com",
		"name":       "cmp",
		"created_at": now,
		"updated_at": now,
	}).Execute(); err != nil {
		b.Fatalf("seed dbx: %v", err)
	}
	return raw, db, id
}

func BenchmarkCompareByPK(b *testing.B) {
	raw, db, id := setupCompareDB(b)
	ctx := context.Background()
	withFreshRegistryB(b)

	b.Run("orm", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, err := ByPK[compareUser](ctx, db, id); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("dbx", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			var out compareUser
			if err := db.Select("id", "email", "name", "created_at", "updated_at").
				From("users").
				Where(dbx.HashExp{"id": id}).
				One(&out); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("database_sql", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			var out compareUser
			row := raw.QueryRowContext(ctx, `SELECT id,email,name,created_at,updated_at FROM users WHERE id = ?`, id)
			if err := row.Scan(&out.ID, &out.Email, &out.Name, &out.CreatedAt, &out.UpdatedAt); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkCompareInsertOne(b *testing.B) {
	raw, db, _ := setupCompareDB(b)
	ctx := context.Background()
	withFreshRegistryB(b)

	b.Run("orm", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			u := compareUser{Email: fmt.Sprintf("orm-%d@example.com", i), Name: "orm"}
			if err := Insert(ctx, db, &u); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("dbx", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			now := time.Now().UTC()
			if _, err := db.Insert("users", dbx.Params{
				"email":      fmt.Sprintf("dbx-%d@example.com", i),
				"name":       "dbx",
				"created_at": now,
				"updated_at": now,
			}).Execute(); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("database_sql", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			now := time.Now().UTC()
			if _, err := raw.ExecContext(ctx, `INSERT INTO users (email,name,created_at,updated_at) VALUES (?,?,?,?)`,
				fmt.Sprintf("sql-%d@example.com", i), "sql", now, now); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkCompareListQuery(b *testing.B) {
	raw, db, _ := setupCompareDB(b)
	ctx := context.Background()
	withFreshRegistryB(b)
	for i := 0; i < 200; i++ {
		now := time.Now().UTC()
		_, _ = raw.ExecContext(ctx, `INSERT INTO users (email,name,created_at,updated_at) VALUES (?,?,?,?)`,
			fmt.Sprintf("seed-sql-%d@example.com", i), "sql", now, now)
		_, _ = db.Insert("users", dbx.Params{
			"email":      fmt.Sprintf("seed-dbx-%d@example.com", i),
			"name":       "dbx",
			"created_at": now,
			"updated_at": now,
		}).Execute()
	}
	b.Run("orm", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			rows, err := Query[compareUser](db).OrderBy("id").Page(1, 50).All(ctx)
			if err != nil {
				b.Fatal(err)
			}
			if len(rows) == 0 {
				b.Fatal("no rows")
			}
		}
	})
	b.Run("dbx", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			var rows []compareUser
			if err := db.Select("id", "email", "name", "created_at", "updated_at").
				From("users").
				OrderBy("id").
				Limit(50).
				All(&rows); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("database_sql", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			rs, err := raw.QueryContext(ctx, `SELECT id,email,name,created_at,updated_at FROM users ORDER BY id LIMIT 50`)
			if err != nil {
				b.Fatal(err)
			}
			for rs.Next() {
				var u compareUser
				if err := rs.Scan(&u.ID, &u.Email, &u.Name, &u.CreatedAt, &u.UpdatedAt); err != nil {
					_ = rs.Close()
					b.Fatal(err)
				}
			}
			_ = rs.Close()
		}
	})
}

func BenchmarkCompareUpdateOne(b *testing.B) {
	raw, db, id := setupCompareDB(b)
	ctx := context.Background()
	withFreshRegistryB(b)

	b.Run("orm", func(b *testing.B) {
		u := compareUser{ID: id, Email: "cmp@example.com", Name: "cmp"}
		for i := 0; i < b.N; i++ {
			u.Name = fmt.Sprintf("orm-u-%d", i)
			if err := Update(ctx, db, &u); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("dbx", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, err := db.Update("users", dbx.Params{
				"name":       fmt.Sprintf("dbx-u-%d", i),
				"updated_at": time.Now().UTC(),
			}, dbx.HashExp{"id": id}).Execute(); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("database_sql", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, err := raw.ExecContext(ctx, `UPDATE users SET name=?, updated_at=? WHERE id=?`,
				fmt.Sprintf("sql-u-%d", i), time.Now().UTC(), id); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkCompareBatchInsert(b *testing.B) {
	raw, db, _ := setupCompareDB(b)
	ctx := context.Background()
	withFreshRegistryB(b)

	b.Run("orm", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			rows := []*compareUser{
				{Email: fmt.Sprintf("orm-b-%d-1@example.com", i), Name: "b1"},
				{Email: fmt.Sprintf("orm-b-%d-2@example.com", i), Name: "b2"},
				{Email: fmt.Sprintf("orm-b-%d-3@example.com", i), Name: "b3"},
			}
			if _, err := InsertBatch(ctx, db, rows); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("dbx", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			now := time.Now().UTC()
			rows := []dbx.Params{
				{"email": fmt.Sprintf("dbx-b-%d-1@example.com", i), "name": "b1", "created_at": now, "updated_at": now},
				{"email": fmt.Sprintf("dbx-b-%d-2@example.com", i), "name": "b2", "created_at": now, "updated_at": now},
				{"email": fmt.Sprintf("dbx-b-%d-3@example.com", i), "name": "b3", "created_at": now, "updated_at": now},
			}
			if _, err := db.InsertMany("users", rows).Execute(); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("database_sql", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			now := time.Now().UTC()
			if _, err := raw.ExecContext(ctx,
				`INSERT INTO users (email,name,created_at,updated_at) VALUES (?,?,?,?),(?,?,?,?),(?,?,?,?)`,
				fmt.Sprintf("sql-b-%d-1@example.com", i), "b1", now, now,
				fmt.Sprintf("sql-b-%d-2@example.com", i), "b2", now, now,
				fmt.Sprintf("sql-b-%d-3@example.com", i), "b3", now, now); err != nil {
				b.Fatal(err)
			}
		}
	})
}
