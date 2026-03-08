package orm

import "testing"

func TestDefaultNamingStrategy(t *testing.T) {
	n := DefaultNamingStrategy{}

	if got := n.TableName("User"); got != "users" {
		t.Fatalf("expected users, got %q", got)
	}
	if got := n.TableName("Category"); got != "categories" {
		t.Fatalf("expected categories, got %q", got)
	}
	if got := n.TableName("Box"); got != "boxes" {
		t.Fatalf("expected boxes, got %q", got)
	}
	if got := n.ColumnName("CreatedAt"); got != "created_at" {
		t.Fatalf("expected created_at, got %q", got)
	}
}
