package orm

import (
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

func wrapQueryError(base *Error, op, model, field string, err error) error {
	if err == nil {
		return nil
	}
	if isConflictError(err) {
		return ErrConflict.with(op, model, field, err)
	}
	return base.with(op, model, field, err)
}

func isConflictError(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		// SQLSTATE class 23: integrity constraint violation.
		return strings.HasPrefix(pgErr.Code, "23")
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "duplicate key") ||
		strings.Contains(msg, "constraint failed") {
		return true
	}
	return false
}
