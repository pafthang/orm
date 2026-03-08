package typemeta

import "testing"

type tmEmbedded struct {
	Code string `db:"code"`
}

type tmUser struct {
	ID   int64  `db:"id,pk"`
	Name string `db:"name" json:"name"`
	tmEmbedded
}

func TestResolve(t *testing.T) {
	r := NewRegistry()
	meta, err := r.Resolve(tmUser{})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if meta.Name != "tmUser" {
		t.Fatalf("unexpected name: %s", meta.Name)
	}
	if len(meta.Fields) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(meta.Fields))
	}
	if !meta.FieldsByGo["ID"].IsPK {
		t.Fatalf("expected ID to be pk")
	}
	if meta.FieldsByGo["Code"] == nil {
		t.Fatalf("expected embedded field")
	}
}
