package orm

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/pafthang/dbx"
	_ "modernc.org/sqlite"
)

type benchUser struct {
	ID        int64       `db:"id,pk"`
	Email     string      `db:"email"`
	Name      string      `db:"name"`
	CreatedAt time.Time   `db:"created_at,created_at"`
	UpdatedAt time.Time   `db:"updated_at,updated_at"`
	Posts     []benchPost `orm:"rel=has_many,local=ID,foreign=UserID"`
}

func (benchUser) TableName() string { return "bench_users" }

type benchPost struct {
	ID        int64     `db:"id,pk"`
	UserID    int64     `db:"user_id"`
	Title     string    `db:"title"`
	CreatedAt time.Time `db:"created_at,created_at"`
}

func (benchPost) TableName() string { return "bench_posts" }

func setupBenchDB(b *testing.B) *dbx.DB {
	b.Helper()
	db, err := dbx.Open("sqlite", ":memory:")
	if err != nil {
		b.Fatalf("open sqlite: %v", err)
	}
	b.Cleanup(func() { _ = db.Close() })

	schema := `
CREATE TABLE bench_users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	email TEXT NOT NULL,
	name TEXT NOT NULL,
	created_at TIMESTAMP NOT NULL,
	updated_at TIMESTAMP NOT NULL
);
CREATE TABLE bench_posts (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL,
	title TEXT NOT NULL,
	created_at TIMESTAMP NOT NULL
);`
	if _, err := db.NewQuery(schema).Execute(); err != nil {
		b.Fatalf("create schema: %v", err)
	}
	return db
}

func seedBenchUsers(b *testing.B, db DB, n int) []benchUser {
	b.Helper()
	ctx := context.Background()
	out := make([]benchUser, n)
	for i := 0; i < n; i++ {
		u := benchUser{
			Email: fmt.Sprintf("u%04d@example.com", i),
			Name:  fmt.Sprintf("User%04d", i),
		}
		if err := Insert(ctx, db, &u); err != nil {
			b.Fatalf("insert seed %d: %v", i, err)
		}
		out[i] = u
	}
	return out
}

func BenchmarkByPK(b *testing.B) {
	withFreshRegistryB(b)
	db := setupBenchDB(b)
	users := seedBenchUsers(b, db, 100)
	ctx := context.Background()
	id := users[len(users)/2].ID
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ByPK[benchUser](ctx, db, id); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkListQuery(b *testing.B) {
	withFreshRegistryB(b)
	db := setupBenchDB(b)
	seedBenchUsers(b, db, 1000)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rows, err := Query[benchUser](db).OrderBy("id").Page(1, 50).All(ctx)
		if err != nil {
			b.Fatal(err)
		}
		if len(rows) != 50 {
			b.Fatalf("unexpected row count: %d", len(rows))
		}
	}
}

func BenchmarkInsertOne(b *testing.B) {
	withFreshRegistryB(b)
	db := setupBenchDB(b)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		u := benchUser{
			Email: fmt.Sprintf("ins%08d@example.com", i),
			Name:  "Insert",
		}
		if err := Insert(ctx, db, &u); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUpdateOne(b *testing.B) {
	withFreshRegistryB(b)
	db := setupBenchDB(b)
	users := seedBenchUsers(b, db, 1)
	ctx := context.Background()
	u := users[0]
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		u.Name = fmt.Sprintf("Updated-%d", i)
		if err := Update(ctx, db, &u); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPreloadRelation(b *testing.B) {
	withFreshRegistryB(b)
	db := setupBenchDB(b)
	users := seedBenchUsers(b, db, 100)
	ctx := context.Background()
	for i := range users {
		p := benchPost{UserID: users[i].ID, Title: "post"}
		if err := Insert(ctx, db, &p); err != nil {
			b.Fatalf("insert post: %v", err)
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rows, err := Query[benchUser](db).Preload("Posts").Page(1, 50).All(ctx)
		if err != nil {
			b.Fatal(err)
		}
		if len(rows) == 0 {
			b.Fatal("empty rows")
		}
	}
}

func BenchmarkBatchInsert(b *testing.B) {
	withFreshRegistryB(b)
	db := setupBenchDB(b)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rows := []benchUser{
			{Email: fmt.Sprintf("b%08d-1@example.com", i), Name: "B1"},
			{Email: fmt.Sprintf("b%08d-2@example.com", i), Name: "B2"},
			{Email: fmt.Sprintf("b%08d-3@example.com", i), Name: "B3"},
		}
		ptrs := []*benchUser{&rows[0], &rows[1], &rows[2]}
		if _, err := InsertBatch(ctx, db, ptrs); err != nil {
			b.Fatal(err)
		}
	}
}

func withFreshRegistryB(b *testing.B) {
	b.Helper()
	r := NewRegistry()
	old := DefaultRegistry
	DefaultRegistry = r
	b.Cleanup(func() { DefaultRegistry = old })
}
