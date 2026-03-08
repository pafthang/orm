package orm

import "testing"

func TestTypeMetadata(t *testing.T) {
	meta, err := TypeMetadata[crudUser]()
	if err != nil {
		t.Fatalf("type metadata: %v", err)
	}
	if meta.FieldsByGo["ID"] == nil {
		t.Fatalf("expected ID field metadata")
	}
}
