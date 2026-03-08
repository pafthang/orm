package orm

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// JSONValue stores arbitrary Go value as JSON in DB.
// It implements sql.Scanner and driver.Valuer.
type JSONValue[T any] struct {
	V T
}

func (j JSONValue[T]) Value() (driver.Value, error) {
	b, err := json.Marshal(j.V)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func (j *JSONValue[T]) Scan(src any) error {
	if j == nil {
		return fmt.Errorf("nil JSONValue receiver")
	}
	switch v := src.(type) {
	case nil:
		var zero T
		j.V = zero
		return nil
	case []byte:
		return json.Unmarshal(v, &j.V)
	case string:
		return json.Unmarshal([]byte(v), &j.V)
	default:
		return fmt.Errorf("unsupported JSON source type: %T", src)
	}
}
