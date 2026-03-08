package orm

import "github.com/pafthang/orm/typemeta"

// SharedTypeRegistry exposes transport-neutral type metadata cache.
var SharedTypeRegistry = typemeta.NewRegistry()

// TypeMetadata returns shared metadata for model T.
func TypeMetadata[T any]() (*typemeta.TypeMeta, error) {
	var zero T
	return SharedTypeRegistry.Resolve(zero)
}
