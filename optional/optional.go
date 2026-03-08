package optional

// Value represents explicit optional value semantics:
// set=true means field is present even when inner value is zero.
type Value[T any] struct {
	Set   bool
	Value T
}

// Some marks value as explicitly set.
func Some[T any](v T) Value[T] { return Value[T]{Set: true, Value: v} }

// None marks value as absent.
func None[T any]() Value[T] { return Value[T]{} }

func (o Value[T]) IsSet() bool   { return o.Set }
func (o Value[T]) ValueAny() any { return o.Value }

// Nullable represents patch semantics with explicit NULL support.
type Nullable[T any] struct {
	Set   bool
	Null  bool
	Value T
}

// SomeNullable marks value as explicitly set to non-null payload.
func SomeNullable[T any](v T) Nullable[T] {
	return Nullable[T]{Set: true, Value: v}
}

// Null marks value as explicitly set to SQL NULL.
func Null[T any]() Nullable[T] {
	return Nullable[T]{Set: true, Null: true}
}

// Unset marks value as absent.
func Unset[T any]() Nullable[T] {
	return Nullable[T]{}
}

func (n Nullable[T]) IsSet() bool   { return n.Set }
func (n Nullable[T]) ValueAny() any { return n.Value }
func (n Nullable[T]) IsNull() bool  { return n.Null }
