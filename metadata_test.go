package orm

import (
	"database/sql/driver"
	"reflect"
	"testing"
	"time"
)

type auditFields struct {
	CreatedAt time.Time `db:"created_at,created_at"`
	UpdatedAt time.Time `db:"updated_at,updated_at"`
}

type userModel struct {
	ID        int64      `db:"id,pk" json:"id"`
	Email     string     `db:"email"`
	Name      string     `db:"name"`
	DeletedAt *time.Time `db:"deleted_at,soft_delete"`
	Password  string     `db:"password,writeonly"`
	Ignored   string     `db:"-"`
	auditFields
}

func (userModel) TableName() string { return "app_users" }

func TestParseModelMeta(t *testing.T) {
	meta, err := parseModelMeta(mustType(userModel{}), ModelConfig{}, DefaultNamingStrategy{})
	if err != nil {
		t.Fatalf("parseModelMeta() error = %v", err)
	}

	if meta.Table != "app_users" {
		t.Fatalf("expected table app_users, got %q", meta.Table)
	}
	if len(meta.PrimaryKeys) != 1 || meta.PrimaryKeys[0].DBName != "id" {
		t.Fatalf("expected single pk id, got %#v", meta.PrimaryKeys)
	}
	if meta.SoftDeleteField == nil || meta.SoftDeleteField.DBName != "deleted_at" {
		t.Fatalf("expected soft delete deleted_at")
	}
	if _, ok := meta.FieldsByGo["Ignored"]; ok {
		t.Fatalf("ignored field should not be present")
	}
	if meta.CreatedAtField == nil || meta.UpdatedAtField == nil {
		t.Fatalf("expected audit timestamps from embedded fields")
	}
	if !meta.FieldsByGo["DeletedAt"].IsNullable {
		t.Fatalf("pointer field should be nullable")
	}
	if !meta.FieldsByGo["Password"].IsWriteOnly {
		t.Fatalf("password should be write-only")
	}
}

func TestParseModelMetaRequiresPK(t *testing.T) {
	type noPK struct {
		Email string `db:"email"`
	}

	_, err := parseModelMeta(mustType(noPK{}), ModelConfig{}, DefaultNamingStrategy{})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !isCode(err, CodeMissingPrimaryKey) {
		t.Fatalf("expected missing_primary_key, got %v", err)
	}
}

type relProfileMeta struct {
	ID  int64  `db:"id,pk"`
	Bio string `db:"bio"`
}

type relOrderMeta struct {
	ID        int64  `db:"id,pk"`
	RelUserID int64  `db:"rel_user_id"`
	Title     string `db:"title"`
}

type relUserMeta struct {
	ID        int64           `db:"id,pk"`
	ProfileID int64           `db:"profile_id"`
	Profile   *relProfileMeta `orm:"rel=belongs_to,local=ProfileID,foreign=ID"`
	Orders    []relOrderMeta
}

func TestInferRelations(t *testing.T) {
	meta, err := parseModelMeta(mustType(relUserMeta{}), ModelConfig{}, DefaultNamingStrategy{})
	if err != nil {
		t.Fatalf("parseModelMeta() error = %v", err)
	}
	if len(meta.Relations) != 2 {
		t.Fatalf("expected 2 relations, got %d", len(meta.Relations))
	}
	profile := meta.Relations["Profile"]
	if profile == nil || profile.Kind != RelationBelongsTo {
		t.Fatalf("expected belongs_to Profile relation")
	}
	orders := meta.Relations["Orders"]
	if orders == nil || orders.Kind != RelationHasMany {
		t.Fatalf("expected has_many Orders relation")
	}
}

func TestRelationTagOverrideInvalid(t *testing.T) {
	type badUser struct {
		ID int64           `db:"id,pk"`
		P  *relProfileMeta `orm:"rel=oops"`
	}
	_, err := parseModelMeta(mustType(badUser{}), ModelConfig{}, DefaultNamingStrategy{})
	if err == nil {
		t.Fatalf("expected error for invalid relation tag")
	}
	if !isCode(err, CodeInvalidModel) {
		t.Fatalf("expected invalid model code, got %v", err)
	}
}

type customCodec string

func (c *customCodec) Scan(src any) error {
	switch v := src.(type) {
	case string:
		*c = customCodec(v)
	}
	return nil
}

func (c customCodec) Value() (driver.Value, error) {
	return string(c), nil
}

func TestSupportedCustomScannerValuerType(t *testing.T) {
	type m struct {
		ID    int64       `db:"id,pk"`
		Codec customCodec `db:"codec"`
	}
	_, err := parseModelMeta(mustType(m{}), ModelConfig{}, DefaultNamingStrategy{})
	if err != nil {
		t.Fatalf("expected custom scanner/valuer type to be supported, got %v", err)
	}
}

func TestUnsupportedFieldType(t *testing.T) {
	type bad struct {
		ID   int64 `db:"id,pk"`
		Meta struct {
			A string
		} `db:"meta"`
	}
	_, err := parseModelMeta(mustType(bad{}), ModelConfig{}, DefaultNamingStrategy{})
	if err == nil {
		t.Fatalf("expected unsupported type error")
	}
	if !isCode(err, CodeUnsupportedType) {
		t.Fatalf("expected unsupported_type code, got %v", err)
	}
}

func mustType(v any) reflect.Type {
	t, err := modelType(v)
	if err != nil {
		panic(err)
	}
	return t
}
