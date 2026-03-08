package optional

import "testing"

func TestValueSomeNone(t *testing.T) {
	v := Some("x")
	if !v.IsSet() || v.ValueAny() != "x" {
		t.Fatalf("unexpected some value: %+v", v)
	}
	n := None[string]()
	if n.IsSet() {
		t.Fatalf("none should be unset")
	}
}

func TestNullableVariants(t *testing.T) {
	a := SomeNullable(10)
	if !a.IsSet() || a.IsNull() || a.ValueAny() != 10 {
		t.Fatalf("unexpected some nullable: %+v", a)
	}
	b := Null[int]()
	if !b.IsSet() || !b.IsNull() {
		t.Fatalf("unexpected null nullable: %+v", b)
	}
	c := Unset[int]()
	if c.IsSet() {
		t.Fatalf("unexpected unset nullable: %+v", c)
	}
}
