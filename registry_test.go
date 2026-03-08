package orm

import (
	"errors"
	"testing"
)

type registryModel struct {
	ID int64 `db:"id,pk"`
}

type registryCompany struct {
	ID   int64  `db:"id,pk"`
	Name string `db:"name"`
}

type registryEmployee struct {
	ID           int64 `db:"id,pk"`
	CompanyRefID int64 `db:"company_ref_id"`
	Company      *registryCompany
	Peers        []registryPeer
}

type registryPeer struct {
	ID             int64 `db:"id,pk"`
	RegistryModelX int64 `db:"registry_model_x"`
}

type registryBadRelationShape struct {
	ID      int64           `db:"id,pk"`
	Company registryCompany `orm:"rel=has_many,local=ID,foreign=ID"`
}

type registryBadKeyTypeTarget struct {
	ID string `db:"id,pk"`
}

type registryBadKeyTypeSource struct {
	ID   int64                      `db:"id,pk"`
	Refs []registryBadKeyTypeTarget `orm:"rel=has_many,local=ID,foreign=ID"`
}

func TestRegistryResolveLazy(t *testing.T) {
	r := NewRegistry()
	meta, err := r.Resolve(registryModel{})
	if err != nil {
		t.Fatalf("resolve error: %v", err)
	}
	if meta.Table != "registry_models" {
		t.Fatalf("expected registry_models, got %q", meta.Table)
	}
}

func TestRegistryValidate(t *testing.T) {
	r := NewRegistry()
	if _, err := r.RegisterType(registryModel{}); err != nil {
		t.Fatalf("register error: %v", err)
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("validate error: %v", err)
	}
}

func TestRegistryRelationOverride(t *testing.T) {
	r := NewRegistry()
	if _, err := r.RegisterType(registryCompany{}); err != nil {
		t.Fatalf("register company: %v", err)
	}
	if _, err := r.RegisterType(registryPeer{}); err != nil {
		t.Fatalf("register peer: %v", err)
	}
	meta, err := r.RegisterType(registryEmployee{}, ModelConfig{
		Relations: map[string]RelationConfig{
			"Company": {
				Kind:       RelationBelongsTo,
				LocalField: "CompanyRefID",
				ForeignRef: "ID",
			},
			"Peers": {
				Kind:       RelationHasMany,
				LocalField: "ID",
				ForeignRef: "RegistryModelX",
			},
		},
	})
	if err != nil {
		t.Fatalf("register employee with override: %v", err)
	}
	if meta.Relations["Company"] == nil || meta.Relations["Company"].LocalField != "CompanyRefID" {
		t.Fatalf("company relation override was not applied")
	}
	if meta.Relations["Peers"] == nil || meta.Relations["Peers"].ForeignRef != "RegistryModelX" {
		t.Fatalf("peers relation override was not applied")
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestRegistryValidateInvalidRelation(t *testing.T) {
	r := NewRegistry()
	if _, err := r.RegisterType(registryCompany{}); err != nil {
		t.Fatalf("register company: %v", err)
	}
	if _, err := r.RegisterType(registryEmployee{}, ModelConfig{
		Relations: map[string]RelationConfig{
			"Company": {
				Kind:       RelationBelongsTo,
				LocalField: "CompanyRefID",
				ForeignRef: "NoSuchField",
			},
		},
	}); err != nil {
		t.Fatalf("register employee: %v", err)
	}
	err := r.Validate()
	if err == nil {
		t.Fatalf("expected validate error")
	}
	if !isCode(err, CodeInvalidModel) {
		t.Fatalf("expected invalid model code, got %v", err)
	}
}

func TestRegistryValidateRelationFieldShape(t *testing.T) {
	r := NewRegistry()
	if _, err := r.RegisterType(registryCompany{}); err != nil {
		t.Fatalf("register company: %v", err)
	}
	if _, err := r.RegisterType(registryBadRelationShape{}); err != nil {
		t.Fatalf("register bad shape model: %v", err)
	}
	err := r.Validate()
	if err == nil {
		t.Fatalf("expected validate error")
	}
	if !isCode(err, CodeInvalidModel) {
		t.Fatalf("expected invalid model code, got %v", err)
	}
}

func TestRegistryValidateRelationKeyTypeMismatch(t *testing.T) {
	r := NewRegistry()
	if _, err := r.RegisterType(registryBadKeyTypeTarget{}); err != nil {
		t.Fatalf("register target: %v", err)
	}
	if _, err := r.RegisterType(registryBadKeyTypeSource{}); err != nil {
		t.Fatalf("register source: %v", err)
	}
	err := r.Validate()
	if err == nil {
		t.Fatalf("expected validate error")
	}
	if !isCode(err, CodeInvalidModel) {
		t.Fatalf("expected invalid model code, got %v", err)
	}
}

func TestErrorIsByCode(t *testing.T) {
	err := ErrInvalidField.with("op", "M", "f", errors.New("bad"))
	if !errors.Is(err, ErrInvalidField) {
		t.Fatalf("expected errors.Is true")
	}
}

func isCode(err error, code ErrorCode) bool {
	var oe *Error
	if !errors.As(err, &oe) {
		return false
	}
	return oe.Code == code
}
