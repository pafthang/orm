package orm

import (
	"context"
	"testing"
	"time"

	"github.com/pafthang/dbx"
	_ "modernc.org/sqlite"
)

type RelProfile struct {
	ID  int64  `db:"id,pk"`
	Bio string `db:"bio"`
}

func (RelProfile) TableName() string { return "rel_profiles" }

type RelOrder struct {
	ID        int64  `db:"id,pk"`
	RelUserID int64  `db:"rel_user_id"`
	ProductID int64  `db:"product_id"`
	Title     string `db:"title"`
	Product   *RelProduct
}

func (RelOrder) TableName() string { return "rel_orders" }

type RelProduct struct {
	ID        int64      `db:"id,pk"`
	Name      string     `db:"name"`
	DeletedAt *time.Time `db:"deleted_at,soft_delete"`
}

func (RelProduct) TableName() string { return "rel_products" }

type RelUser struct {
	ID        int64  `db:"id,pk"`
	ProfileID int64  `db:"profile_id"`
	Email     string `db:"email"`
	Profile   *RelProfile
	Orders    []RelOrder
	Roles     []RelRole `orm:"rel=many_to_many,local=ID,foreign=ID,join_table=rel_user_roles,join_local=user_id,join_foreign=role_id"`
}

func (RelUser) TableName() string { return "rel_users" }

type RelRole struct {
	ID   int64  `db:"id,pk"`
	Name string `db:"name"`
}

func (RelRole) TableName() string { return "rel_roles" }

func setupPreloadDB(t *testing.T) *dbx.DB {
	t.Helper()
	db, err := dbx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	schema := `
CREATE TABLE rel_profiles (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	bio TEXT NOT NULL
);
CREATE TABLE rel_users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	profile_id INTEGER NOT NULL,
	email TEXT NOT NULL
);
CREATE TABLE rel_orders (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	rel_user_id INTEGER NOT NULL,
	product_id INTEGER NOT NULL,
	title TEXT NOT NULL
);
CREATE TABLE rel_products (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	deleted_at TIMESTAMP NULL
);
CREATE TABLE rel_roles (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL
);
CREATE TABLE rel_user_roles (
	user_id INTEGER NOT NULL,
	role_id INTEGER NOT NULL
);`
	if _, err := db.NewQuery(schema).Execute(); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return db
}

func TestPreloadBelongsToAndHasMany(t *testing.T) {
	withFreshRegistry(t)
	db := setupPreloadDB(t)
	ctx := context.Background()

	p1 := RelProfile{Bio: "p1"}
	p2 := RelProfile{Bio: "p2"}
	if err := Insert(ctx, db, &p1); err != nil {
		t.Fatalf("insert p1: %v", err)
	}
	if err := Insert(ctx, db, &p2); err != nil {
		t.Fatalf("insert p2: %v", err)
	}

	u1 := RelUser{ProfileID: p1.ID, Email: "u1@example.com"}
	u2 := RelUser{ProfileID: p2.ID, Email: "u2@example.com"}
	if err := Insert(ctx, db, &u1); err != nil {
		t.Fatalf("insert u1: %v", err)
	}
	if err := Insert(ctx, db, &u2); err != nil {
		t.Fatalf("insert u2: %v", err)
	}

	o1 := RelOrder{RelUserID: u1.ID, Title: "o1"}
	o2 := RelOrder{RelUserID: u1.ID, Title: "o2"}
	o3 := RelOrder{RelUserID: u2.ID, Title: "o3"}
	pr1 := RelProduct{Name: "book"}
	pr2 := RelProduct{Name: "pen"}
	if err := Insert(ctx, db, &pr1); err != nil {
		t.Fatalf("insert pr1: %v", err)
	}
	if err := Insert(ctx, db, &pr2); err != nil {
		t.Fatalf("insert pr2: %v", err)
	}
	o1.ProductID = pr1.ID
	o2.ProductID = pr2.ID
	o3.ProductID = pr1.ID
	if err := Insert(ctx, db, &o1); err != nil {
		t.Fatalf("insert o1: %v", err)
	}
	if err := Insert(ctx, db, &o2); err != nil {
		t.Fatalf("insert o2: %v", err)
	}
	if err := Insert(ctx, db, &o3); err != nil {
		t.Fatalf("insert o3: %v", err)
	}

	users, err := Query[RelUser](db).
		OrderBy("id").
		Preload("Profile").
		Preload("Orders").
		Preload("Orders.Product").
		All(ctx)
	if err != nil {
		t.Fatalf("query with preload: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}
	if users[0].Profile == nil || users[0].Profile.Bio != "p1" {
		t.Fatalf("unexpected preloaded profile for user1: %+v", users[0].Profile)
	}
	if users[1].Profile == nil || users[1].Profile.Bio != "p2" {
		t.Fatalf("unexpected preloaded profile for user2: %+v", users[1].Profile)
	}
	if len(users[0].Orders) != 2 {
		t.Fatalf("expected 2 preloaded orders for user1, got %d", len(users[0].Orders))
	}
	if len(users[1].Orders) != 1 {
		t.Fatalf("expected 1 preloaded order for user2, got %d", len(users[1].Orders))
	}
	if users[0].Orders[0].Product == nil || users[0].Orders[0].Product.Name == "" {
		t.Fatalf("expected nested preload Orders.Product for user1")
	}
	if users[1].Orders[0].Product == nil || users[1].Orders[0].Product.Name == "" {
		t.Fatalf("expected nested preload Orders.Product for user2")
	}
}

func TestPreloadWithDeletedOnNestedRelation(t *testing.T) {
	withFreshRegistry(t)
	db := setupPreloadDB(t)
	ctx := context.Background()

	p := RelProfile{Bio: "p"}
	if err := Insert(ctx, db, &p); err != nil {
		t.Fatalf("insert profile: %v", err)
	}
	prVisible := RelProduct{Name: "visible"}
	prDeleted := RelProduct{Name: "deleted"}
	if err := Insert(ctx, db, &prVisible); err != nil {
		t.Fatalf("insert visible product: %v", err)
	}
	if err := Insert(ctx, db, &prDeleted); err != nil {
		t.Fatalf("insert deleted product: %v", err)
	}
	if err := Delete(ctx, db, &prDeleted); err != nil {
		t.Fatalf("soft delete product: %v", err)
	}

	u := RelUser{ProfileID: p.ID, Email: "u@example.com"}
	if err := Insert(ctx, db, &u); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	o1 := RelOrder{RelUserID: u.ID, ProductID: prVisible.ID, Title: "o1"}
	o2 := RelOrder{RelUserID: u.ID, ProductID: prDeleted.ID, Title: "o2"}
	if err := Insert(ctx, db, &o1); err != nil {
		t.Fatalf("insert o1: %v", err)
	}
	if err := Insert(ctx, db, &o2); err != nil {
		t.Fatalf("insert o2: %v", err)
	}

	row, err := Query[RelUser](db).
		WhereEq("id", u.ID).
		Preload("Orders").
		Preload("Orders.Product").
		One(ctx)
	if err != nil {
		t.Fatalf("query without deleted preload: %v", err)
	}
	if len(row.Orders) != 2 {
		t.Fatalf("expected 2 orders, got %d", len(row.Orders))
	}
	if row.Orders[1].Product != nil {
		t.Fatalf("deleted nested product must be hidden by default")
	}

	rowWithDeleted, err := Query[RelUser](db).
		WhereEq("id", u.ID).
		Preload("Orders").
		Preload("Orders.Product", PreloadWithDeleted()).
		One(ctx)
	if err != nil {
		t.Fatalf("query with deleted preload: %v", err)
	}
	if rowWithDeleted.Orders[1].Product == nil {
		t.Fatalf("deleted nested product should be loaded with PreloadWithDeleted")
	}
}

func TestPreloadWhereEq(t *testing.T) {
	withFreshRegistry(t)
	db := setupPreloadDB(t)
	ctx := context.Background()

	p := RelProfile{Bio: "p"}
	if err := Insert(ctx, db, &p); err != nil {
		t.Fatalf("insert profile: %v", err)
	}
	u := RelUser{ProfileID: p.ID, Email: "u@example.com"}
	if err := Insert(ctx, db, &u); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	o1 := RelOrder{RelUserID: u.ID, Title: "first", ProductID: 1}
	o2 := RelOrder{RelUserID: u.ID, Title: "second", ProductID: 1}
	if _, err := db.NewQuery("INSERT INTO rel_products(name) VALUES ('p')").Execute(); err != nil {
		t.Fatalf("insert raw product: %v", err)
	}
	if err := Insert(ctx, db, &o1); err != nil {
		t.Fatalf("insert o1: %v", err)
	}
	if err := Insert(ctx, db, &o2); err != nil {
		t.Fatalf("insert o2: %v", err)
	}

	row, err := Query[RelUser](db).
		WhereEq("id", u.ID).
		Preload("Orders", PreloadWhereEq("Title", "first")).
		One(ctx)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(row.Orders) != 1 || row.Orders[0].Title != "first" {
		t.Fatalf("expected filtered preloaded orders")
	}
}

func TestPreloadWhereEqInvalidField(t *testing.T) {
	withFreshRegistry(t)
	db := setupPreloadDB(t)
	ctx := context.Background()

	p := RelProfile{Bio: "p"}
	if err := Insert(ctx, db, &p); err != nil {
		t.Fatalf("insert profile: %v", err)
	}
	u := RelUser{ProfileID: p.ID, Email: "u@example.com"}
	if err := Insert(ctx, db, &u); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	_, err := Query[RelUser](db).
		WhereEq("id", u.ID).
		Preload("Orders", PreloadWhereEq("NoSuchField", "x")).
		One(ctx)
	if err == nil {
		t.Fatalf("expected preload field error")
	}
	if !isCode(err, CodeInvalidField) {
		t.Fatalf("expected invalid field code, got %v", err)
	}
}

func TestPreloadOrderByField(t *testing.T) {
	withFreshRegistry(t)
	db := setupPreloadDB(t)
	ctx := context.Background()

	p := RelProfile{Bio: "p"}
	if err := Insert(ctx, db, &p); err != nil {
		t.Fatalf("insert profile: %v", err)
	}
	if _, err := db.NewQuery("INSERT INTO rel_products(name) VALUES ('p')").Execute(); err != nil {
		t.Fatalf("insert raw product: %v", err)
	}
	u := RelUser{ProfileID: p.ID, Email: "u@example.com"}
	if err := Insert(ctx, db, &u); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	o1 := RelOrder{RelUserID: u.ID, Title: "B", ProductID: 1}
	o2 := RelOrder{RelUserID: u.ID, Title: "A", ProductID: 1}
	if err := Insert(ctx, db, &o1); err != nil {
		t.Fatalf("insert o1: %v", err)
	}
	if err := Insert(ctx, db, &o2); err != nil {
		t.Fatalf("insert o2: %v", err)
	}

	row, err := Query[RelUser](db).
		WhereEq("id", u.ID).
		Preload("Orders", PreloadOrderByField("Title", SortAsc)).
		One(ctx)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(row.Orders) != 2 {
		t.Fatalf("expected 2 orders")
	}
	if row.Orders[0].Title != "A" || row.Orders[1].Title != "B" {
		t.Fatalf("unexpected order after preload sort: %+v", row.Orders)
	}
}

func TestPreloadOrderByFieldInvalidField(t *testing.T) {
	withFreshRegistry(t)
	db := setupPreloadDB(t)
	ctx := context.Background()

	p := RelProfile{Bio: "p"}
	if err := Insert(ctx, db, &p); err != nil {
		t.Fatalf("insert profile: %v", err)
	}
	u := RelUser{ProfileID: p.ID, Email: "u@example.com"}
	if err := Insert(ctx, db, &u); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	_, err := Query[RelUser](db).
		WhereEq("id", u.ID).
		Preload("Orders", PreloadOrderByField("NoSuchField", SortDesc)).
		One(ctx)
	if err == nil {
		t.Fatalf("expected preload order field error")
	}
	if !isCode(err, CodeInvalidField) {
		t.Fatalf("expected invalid field code, got %v", err)
	}
}

func TestPreloadConfigureAndAdvancedFilters(t *testing.T) {
	withFreshRegistry(t)
	db := setupPreloadDB(t)
	ctx := context.Background()

	p := RelProfile{Bio: "p"}
	if err := Insert(ctx, db, &p); err != nil {
		t.Fatalf("insert profile: %v", err)
	}
	if _, err := db.NewQuery("INSERT INTO rel_products(name) VALUES ('p')").Execute(); err != nil {
		t.Fatalf("insert raw product: %v", err)
	}
	u := RelUser{ProfileID: p.ID, Email: "u@example.com"}
	if err := Insert(ctx, db, &u); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	o1 := RelOrder{RelUserID: u.ID, Title: "alpha", ProductID: 1}
	o2 := RelOrder{RelUserID: u.ID, Title: "beta", ProductID: 1}
	o3 := RelOrder{RelUserID: u.ID, Title: "gamma", ProductID: 1}
	if err := Insert(ctx, db, &o1); err != nil {
		t.Fatalf("insert o1: %v", err)
	}
	if err := Insert(ctx, db, &o2); err != nil {
		t.Fatalf("insert o2: %v", err)
	}
	if err := Insert(ctx, db, &o3); err != nil {
		t.Fatalf("insert o3: %v", err)
	}

	row, err := Query[RelUser](db).
		WhereEq("id", u.ID).
		Preload("Orders", PreloadConfigure(func(p *PreloadScope) {
			p.WhereIn("Title", []string{"alpha", "gamma"})
			p.WhereLike("Title", "a")
			p.WhereGTE("ID", o1.ID)
			p.OrderByField("ID", SortDesc)
			p.Limit(1)
		})).
		One(ctx)
	if err != nil {
		t.Fatalf("query with configured preload: %v", err)
	}
	if len(row.Orders) != 1 {
		t.Fatalf("expected 1 filtered order, got %d", len(row.Orders))
	}
	if row.Orders[0].Title != "gamma" {
		t.Fatalf("expected top desc filtered order 'gamma', got %q", row.Orders[0].Title)
	}
}

func TestPreloadConfigureInvalidField(t *testing.T) {
	withFreshRegistry(t)
	db := setupPreloadDB(t)
	ctx := context.Background()

	p := RelProfile{Bio: "p"}
	if err := Insert(ctx, db, &p); err != nil {
		t.Fatalf("insert profile: %v", err)
	}
	u := RelUser{ProfileID: p.ID, Email: "u@example.com"}
	if err := Insert(ctx, db, &u); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	_, err := Query[RelUser](db).
		WhereEq("id", u.ID).
		Preload("Orders", PreloadConfigure(func(p *PreloadScope) {
			p.WhereGT("NoSuchField", 1)
		})).
		One(ctx)
	if err == nil {
		t.Fatalf("expected preload configure invalid field error")
	}
	if !isCode(err, CodeInvalidField) {
		t.Fatalf("expected invalid field code, got %v", err)
	}
}

func TestUnsafePreloadOptions(t *testing.T) {
	withFreshRegistry(t)
	db := setupPreloadDB(t)
	ctx := context.Background()

	p := RelProfile{Bio: "p"}
	if err := Insert(ctx, db, &p); err != nil {
		t.Fatalf("insert profile: %v", err)
	}
	if _, err := db.NewQuery("INSERT INTO rel_products(name) VALUES ('p')").Execute(); err != nil {
		t.Fatalf("insert raw product: %v", err)
	}
	u := RelUser{ProfileID: p.ID, Email: "u@example.com"}
	if err := Insert(ctx, db, &u); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	o1 := RelOrder{RelUserID: u.ID, Title: "A", ProductID: 1}
	o2 := RelOrder{RelUserID: u.ID, Title: "B", ProductID: 1}
	if err := Insert(ctx, db, &o1); err != nil {
		t.Fatalf("insert o1: %v", err)
	}
	if err := Insert(ctx, db, &o2); err != nil {
		t.Fatalf("insert o2: %v", err)
	}

	row, err := Query[RelUser](db).
		WhereEq("id", u.ID).
		Preload("Orders",
			UnsafePreloadWhereExpr("title = {:t}", dbx.Params{"t": "B"}),
			UnsafePreloadOrderBy("id DESC"),
		).
		One(ctx)
	if err != nil {
		t.Fatalf("query unsafe preload options: %v", err)
	}
	if len(row.Orders) != 1 || row.Orders[0].Title != "B" {
		t.Fatalf("unexpected unsafe preload result: %+v", row.Orders)
	}
}

func TestJoinRelation(t *testing.T) {
	withFreshRegistry(t)
	db := setupPreloadDB(t)
	ctx := context.Background()

	p1 := RelProfile{Bio: "bio-1"}
	p2 := RelProfile{Bio: "bio-2"}
	if err := Insert(ctx, db, &p1); err != nil {
		t.Fatalf("insert p1: %v", err)
	}
	if err := Insert(ctx, db, &p2); err != nil {
		t.Fatalf("insert p2: %v", err)
	}
	u1 := RelUser{ProfileID: p1.ID, Email: "u1@example.com"}
	u2 := RelUser{ProfileID: p2.ID, Email: "u2@example.com"}
	if err := Insert(ctx, db, &u1); err != nil {
		t.Fatalf("insert u1: %v", err)
	}
	if err := Insert(ctx, db, &u2); err != nil {
		t.Fatalf("insert u2: %v", err)
	}

	rows, err := Query[RelUser](db).
		JoinRelation("Profile").
		WhereExpr("rel_profiles.bio = {:bio}", dbx.Params{"bio": "bio-1"}).
		All(ctx)
	if err != nil {
		t.Fatalf("join relation query: %v", err)
	}
	if len(rows) != 1 || rows[0].Email != "u1@example.com" {
		t.Fatalf("unexpected joined result: %+v", rows)
	}
}

func TestRelationFieldFiltersAndOrder(t *testing.T) {
	withFreshRegistry(t)
	db := setupPreloadDB(t)
	ctx := context.Background()

	p1 := RelProfile{Bio: "alpha-bio"}
	p2 := RelProfile{Bio: "beta-bio"}
	if err := Insert(ctx, db, &p1); err != nil {
		t.Fatalf("insert p1: %v", err)
	}
	if err := Insert(ctx, db, &p2); err != nil {
		t.Fatalf("insert p2: %v", err)
	}
	u1 := RelUser{ProfileID: p1.ID, Email: "c@example.com"}
	u2 := RelUser{ProfileID: p2.ID, Email: "a@example.com"}
	if err := Insert(ctx, db, &u1); err != nil {
		t.Fatalf("insert u1: %v", err)
	}
	if err := Insert(ctx, db, &u2); err != nil {
		t.Fatalf("insert u2: %v", err)
	}

	rows, err := Query[RelUser](db).
		WhereRelationLike("Profile.Bio", "beta").
		All(ctx)
	if err != nil {
		t.Fatalf("where relation like: %v", err)
	}
	if len(rows) != 1 || rows[0].Email != "a@example.com" {
		t.Fatalf("unexpected relation-like result: %+v", rows)
	}

	rows, err = Query[RelUser](db).
		WhereRelationIn("Profile.ID", []int64{p1.ID, p2.ID}).
		OrderByRelation("Profile.Bio").
		All(ctx)
	if err != nil {
		t.Fatalf("relation in/order query: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].Email != "c@example.com" {
		t.Fatalf("expected alpha profile first, got %+v", rows)
	}
}

func TestPreloadManyToMany(t *testing.T) {
	withFreshRegistry(t)
	db := setupPreloadDB(t)
	ctx := context.Background()

	p := RelProfile{Bio: "p"}
	if err := Insert(ctx, db, &p); err != nil {
		t.Fatalf("insert profile: %v", err)
	}
	u1 := RelUser{ProfileID: p.ID, Email: "u1@example.com"}
	u2 := RelUser{ProfileID: p.ID, Email: "u2@example.com"}
	if err := Insert(ctx, db, &u1); err != nil {
		t.Fatalf("insert u1: %v", err)
	}
	if err := Insert(ctx, db, &u2); err != nil {
		t.Fatalf("insert u2: %v", err)
	}

	r1 := RelRole{Name: "admin"}
	r2 := RelRole{Name: "editor"}
	if err := Insert(ctx, db, &r1); err != nil {
		t.Fatalf("insert r1: %v", err)
	}
	if err := Insert(ctx, db, &r2); err != nil {
		t.Fatalf("insert r2: %v", err)
	}

	if _, err := db.NewQuery("INSERT INTO rel_user_roles(user_id, role_id) VALUES ({:u},{:r})").
		Bind(dbx.Params{"u": u1.ID, "r": r1.ID}).Execute(); err != nil {
		t.Fatalf("insert link 1: %v", err)
	}
	if _, err := db.NewQuery("INSERT INTO rel_user_roles(user_id, role_id) VALUES ({:u},{:r})").
		Bind(dbx.Params{"u": u1.ID, "r": r2.ID}).Execute(); err != nil {
		t.Fatalf("insert link 2: %v", err)
	}
	if _, err := db.NewQuery("INSERT INTO rel_user_roles(user_id, role_id) VALUES ({:u},{:r})").
		Bind(dbx.Params{"u": u2.ID, "r": r2.ID}).Execute(); err != nil {
		t.Fatalf("insert link 3: %v", err)
	}

	users, err := Query[RelUser](db).OrderBy("id").Preload("Roles").All(ctx)
	if err != nil {
		t.Fatalf("query preload roles: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}
	if len(users[0].Roles) != 2 {
		t.Fatalf("expected 2 roles for user1, got %d", len(users[0].Roles))
	}
	if len(users[1].Roles) != 1 {
		t.Fatalf("expected 1 role for user2, got %d", len(users[1].Roles))
	}
}
