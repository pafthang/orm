package orm

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

type tagExtender struct{}

func (tagExtender) ExtendField(sf reflect.StructField, field *FieldMeta) error {
	tag := strings.TrimSpace(sf.Tag.Get("ormx"))
	if tag == "" {
		return nil
	}
	for _, part := range strings.Split(tag, ",") {
		switch strings.TrimSpace(part) {
		case "readonly":
			field.IsReadOnly = true
		case "writeonly":
			field.IsWriteOnly = true
		case "pk":
			field.IsPK = true
		case "":
		default:
			return fmt.Errorf("unknown ormx flag: %s", part)
		}
	}
	return nil
}

func TestFieldExtenderGlobalOption(t *testing.T) {
	type m struct {
		ID   int64  `db:"id" ormx:"pk"`
		Name string `db:"name" ormx:"readonly"`
	}

	r := NewRegistry(WithFieldExtender(tagExtender{}))
	meta, err := r.RegisterType(m{})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if len(meta.PrimaryKeys) != 1 || meta.PrimaryKeys[0].GoName != "ID" {
		t.Fatalf("expected pk from extender, got %+v", meta.PrimaryKeys)
	}
	if !meta.FieldsByGo["Name"].IsReadOnly {
		t.Fatalf("expected readonly from extender")
	}
}

func TestFieldExtenderModelConfig(t *testing.T) {
	type m struct {
		ID    int64  `db:"id,pk"`
		Token string `db:"token" ormx:"writeonly"`
	}
	r := NewRegistry()
	meta, err := r.RegisterType(m{}, ModelConfig{Extenders: []FieldExtender{tagExtender{}}})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if !meta.FieldsByGo["Token"].IsWriteOnly {
		t.Fatalf("expected writeonly from model extender")
	}
}
