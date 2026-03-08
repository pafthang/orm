package orm

import (
	"errors"
	"testing"
)

func TestErrorModelStableAccessors(t *testing.T) {
	err := ErrInvalidField.with("resolve", "User", "Email", errors.New("bad field"))

	oe, ok := AsError(err)
	if !ok {
		t.Fatalf("expected AsError=true")
	}
	if oe.ErrorCode() != CodeInvalidField {
		t.Fatalf("expected invalid_field code, got %s", oe.ErrorCode())
	}
	d := oe.Details()
	if d.Op != "resolve" || d.Model != "User" || d.Field != "Email" {
		t.Fatalf("unexpected details: %+v", d)
	}
	if !HasCode(err, CodeInvalidField) {
		t.Fatalf("expected HasCode=true")
	}
	if HasCode(err, CodeNotFound) {
		t.Fatalf("expected HasCode=false for other code")
	}
}
