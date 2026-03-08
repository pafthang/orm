package orm

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/pafthang/dbx"
)

type encodedString struct {
	V string
}

type encodedStringCodec struct{}

func (encodedStringCodec) Encode(value any) (any, error) {
	v, ok := value.(encodedString)
	if !ok {
		return nil, fmt.Errorf("unexpected value type: %T", value)
	}
	return strings.ToUpper(v.V), nil
}

func (encodedStringCodec) Decode(value any) (any, error) { return value, nil }

type stringCodec struct{}

func (stringCodec) Encode(value any) (any, error) {
	s, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("unexpected type: %T", value)
	}
	return strings.ToUpper(s), nil
}
func (stringCodec) Decode(value any) (any, error) {
	s, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("unexpected type: %T", value)
	}
	return strings.ToLower(s), nil
}

type codecModel struct {
	ID   int64         `db:"id,pk"`
	Data encodedString `db:"data"`
}

func (codecModel) TableName() string { return "codec_models" }

func setupCodecDB(t *testing.T) DB {
	t.Helper()
	db := setupSQLiteDB(t)
	schema := `
DROP TABLE users;
CREATE TABLE codec_models (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	data TEXT NOT NULL
);`
	if _, err := db.NewQuery(schema).Execute(); err != nil {
		t.Fatalf("create codec schema: %v", err)
	}
	return db
}

func TestCustomCodecEncodeOnInsert(t *testing.T) {
	withFreshRegistry(t)
	ResetCodecs()
	t.Cleanup(ResetCodecs)
	if err := RegisterCodec[encodedString](encodedStringCodec{}); err != nil {
		t.Fatalf("register codec: %v", err)
	}

	db := setupCodecDB(t)
	ctx := context.Background()

	row := codecModel{Data: encodedString{V: "hello"}}
	if err := Insert(ctx, db, &row); err != nil {
		t.Fatalf("insert with codec: %v", err)
	}

	var got string
	if err := db.Select("data").From("codec_models").Where(dbx.HashExp{"id": row.ID}).One(&got); err != nil {
		t.Fatalf("fetch raw data: %v", err)
	}
	if got != "HELLO" {
		t.Fatalf("expected encoded value HELLO, got %q", got)
	}
}

type codecReadModel struct {
	ID   int64  `db:"id,pk"`
	Name string `db:"name"`
}

func (codecReadModel) TableName() string { return "codec_read_models" }

func TestCodecDecodeOnRead(t *testing.T) {
	withFreshRegistry(t)
	ResetCodecs()
	t.Cleanup(ResetCodecs)
	if err := RegisterCodec[string](stringCodec{}); err != nil {
		t.Fatalf("register string codec: %v", err)
	}

	db := setupSQLiteDB(t)
	if _, err := db.NewQuery(`
DROP TABLE users;
CREATE TABLE codec_read_models (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL
);`).Execute(); err != nil {
		t.Fatalf("schema: %v", err)
	}
	ctx := context.Background()
	row := codecReadModel{Name: "alice"}
	if err := Insert(ctx, db, &row); err != nil {
		t.Fatalf("insert: %v", err)
	}
	got, err := ByPK[codecReadModel](ctx, db, row.ID)
	if err != nil {
		t.Fatalf("by pk: %v", err)
	}
	if got.Name != "alice" {
		t.Fatalf("expected decoded lowercase name, got %q", got.Name)
	}
}

type rawBlob struct {
	S string
}

type rawBlobCodec struct{}

func (rawBlobCodec) Encode(value any) (any, error) {
	v, ok := value.(rawBlob)
	if !ok {
		return nil, fmt.Errorf("unexpected type: %T", value)
	}
	return v.S, nil
}
func (rawBlobCodec) Decode(value any) (any, error) {
	s, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("unexpected type: %T", value)
	}
	return rawBlob{S: s}, nil
}

func TestFieldLevelCodecForUnsupportedType(t *testing.T) {
	type m struct {
		ID   int64   `db:"id,pk"`
		Blob rawBlob `db:"blob"`
	}
	withFreshRegistry(t)
	if _, err := DefaultRegistry.RegisterType(m{}, ModelConfig{
		Table:       "codec_blob_models",
		FieldCodecs: map[string]Codec{"Blob": rawBlobCodec{}},
	}); err != nil {
		t.Fatalf("register with field codec: %v", err)
	}
	db := setupSQLiteDB(t)
	if _, err := db.NewQuery(`
DROP TABLE users;
CREATE TABLE codec_blob_models (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	blob TEXT NOT NULL
);`).Execute(); err != nil {
		t.Fatalf("schema: %v", err)
	}
	ctx := context.Background()
	row := m{Blob: rawBlob{S: "v1"}}
	if err := Insert(ctx, db, &row); err != nil {
		t.Fatalf("insert: %v", err)
	}
	var raw string
	if err := db.Select("blob").From("codec_blob_models").Where(dbx.HashExp{"id": row.ID}).One(&raw); err != nil {
		t.Fatalf("fetch raw: %v", err)
	}
	if raw != "v1" {
		t.Fatalf("expected encoded blob v1, got %q", raw)
	}
}
