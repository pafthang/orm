package orm

import (
	"errors"
	"fmt"
)

// ErrorCode identifies stable ORM error categories.
type ErrorCode string

const (
	CodeNotFound          ErrorCode = "not_found"
	CodeMultipleRows      ErrorCode = "multiple_rows"
	CodeConflict          ErrorCode = "conflict"
	CodeInvalidModel      ErrorCode = "invalid_model"
	CodeMissingPrimaryKey ErrorCode = "missing_primary_key"
	CodeNoRowsAffected    ErrorCode = "no_rows_affected"
	CodeRelationNotFound  ErrorCode = "relation_not_found"
	CodeUnsupportedType   ErrorCode = "unsupported_type"
	CodeInvalidField      ErrorCode = "invalid_field"
	CodeInvalidQuery      ErrorCode = "invalid_query"
	CodeSoftDeleted       ErrorCode = "soft_deleted"
)

// Error is a machine-readable ORM error.
type Error struct {
	Code  ErrorCode
	Op    string
	Model string
	Field string
	Err   error
}

// ErrorDetails is a stable, machine-readable error context payload.
type ErrorDetails struct {
	Op    string
	Model string
	Field string
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	base := string(e.Code)
	if e.Op != "" {
		base = e.Op + ": " + base
	}
	if e.Model != "" {
		base += " model=" + e.Model
	}
	if e.Field != "" {
		base += " field=" + e.Field
	}
	if e.Err != nil {
		base += ": " + e.Err.Error()
	}
	return base
}

func (e *Error) Unwrap() error { return e.Err }

func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	if !ok {
		return false
	}
	return e.Code != "" && e.Code == t.Code
}

func (e *Error) with(op, model, field string, err error) *Error {
	return &Error{Code: e.Code, Op: op, Model: model, Field: field, Err: err}
}

// ErrorCode returns stable category code.
func (e *Error) ErrorCode() ErrorCode {
	if e == nil {
		return ""
	}
	return e.Code
}

// Details returns structured context payload.
func (e *Error) Details() ErrorDetails {
	if e == nil {
		return ErrorDetails{}
	}
	return ErrorDetails{
		Op:    e.Op,
		Model: e.Model,
		Field: e.Field,
	}
}

var (
	ErrNotFound          = &Error{Code: CodeNotFound}
	ErrMultipleRows      = &Error{Code: CodeMultipleRows}
	ErrConflict          = &Error{Code: CodeConflict}
	ErrInvalidModel      = &Error{Code: CodeInvalidModel}
	ErrMissingPrimaryKey = &Error{Code: CodeMissingPrimaryKey}
	ErrNoRowsAffected    = &Error{Code: CodeNoRowsAffected}
	ErrRelationNotFound  = &Error{Code: CodeRelationNotFound}
	ErrUnsupportedType   = &Error{Code: CodeUnsupportedType}
	ErrInvalidField      = &Error{Code: CodeInvalidField}
	ErrInvalidQuery      = &Error{Code: CodeInvalidQuery}
	ErrSoftDeleted       = &Error{Code: CodeSoftDeleted}
)

func wrapf(base *Error, op, model, field, format string, args ...any) error {
	return base.with(op, model, field, fmt.Errorf(format, args...))
}

// AsError unwraps ORM error from arbitrary wrapped error.
func AsError(err error) (*Error, bool) {
	var oe *Error
	if !errors.As(err, &oe) {
		return nil, false
	}
	return oe, true
}

// HasCode checks whether wrapped error contains target ORM code.
func HasCode(err error, code ErrorCode) bool {
	oe, ok := AsError(err)
	if !ok {
		return false
	}
	return oe.Code == code
}
