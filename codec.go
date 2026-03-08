package orm

import (
	"fmt"
	"reflect"
	"sync"
)

// Codec transforms field values between model and DB representations.
type Codec interface {
	Encode(value any) (any, error)
	Decode(value any) (any, error)
}

var codecState struct {
	mu    sync.RWMutex
	codec map[reflect.Type]Codec
}

// RegisterCodec registers codec for Go type T.
func RegisterCodec[T any](c Codec) error {
	if c == nil {
		return ErrInvalidQuery.with("register_codec", "", "", fmt.Errorf("codec is nil"))
	}
	var zero T
	t := reflect.TypeOf(zero)
	if t == nil {
		return ErrInvalidQuery.with("register_codec", "", "", fmt.Errorf("type cannot be nil"))
	}
	codecState.mu.Lock()
	if codecState.codec == nil {
		codecState.codec = map[reflect.Type]Codec{}
	}
	codecState.codec[t] = c
	codecState.mu.Unlock()
	return nil
}

// ResetCodecs clears codec registry.
func ResetCodecs() {
	codecState.mu.Lock()
	codecState.codec = nil
	codecState.mu.Unlock()
}

func codecForType(t reflect.Type) (Codec, bool) {
	if t == nil {
		return nil, false
	}
	codecState.mu.RLock()
	defer codecState.mu.RUnlock()
	if c, ok := codecState.codec[t]; ok {
		return c, true
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
		if c, ok := codecState.codec[t]; ok {
			return c, true
		}
	}
	return nil, false
}

func hasCodecForType(t reflect.Type) bool {
	_, ok := codecForType(t)
	return ok
}

func codecForField(f *FieldMeta) (Codec, bool) {
	if f != nil && f.Codec != nil {
		return f.Codec, true
	}
	if f == nil {
		return nil, false
	}
	return codecForType(f.Type)
}

func encodeFieldValue(f *FieldMeta, value any) (any, error) {
	c, ok := codecForField(f)
	if !ok {
		return value, nil
	}
	encoded, err := c.Encode(value)
	if err != nil {
		return nil, ErrInvalidQuery.with("codec_encode", "", "", err)
	}
	return encoded, nil
}

func decodeModelValue(meta *ModelMeta, rv reflect.Value) error {
	if meta == nil || !rv.IsValid() {
		return nil
	}
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil
	}
	for _, f := range meta.Fields {
		c, ok := codecForField(f)
		if !ok {
			continue
		}
		fv := rv.FieldByIndex(f.Index)
		if !fv.IsValid() || !fv.CanSet() || !fv.CanInterface() {
			continue
		}
		decoded, err := c.Decode(fv.Interface())
		if err != nil {
			return ErrInvalidQuery.with("codec_decode", meta.Name, f.GoName, err)
		}
		if err := assignDecodedValue(fv, decoded); err != nil {
			return ErrInvalidQuery.with("codec_decode", meta.Name, f.GoName, err)
		}
	}
	return nil
}

func assignDecodedValue(dst reflect.Value, decoded any) error {
	if !dst.CanSet() {
		return fmt.Errorf("destination is not settable")
	}
	if decoded == nil {
		dst.Set(reflect.Zero(dst.Type()))
		return nil
	}
	src := reflect.ValueOf(decoded)
	if src.Type().AssignableTo(dst.Type()) {
		dst.Set(src)
		return nil
	}
	if src.Type().ConvertibleTo(dst.Type()) {
		dst.Set(src.Convert(dst.Type()))
		return nil
	}
	if dst.Kind() == reflect.Pointer {
		elem := dst.Type().Elem()
		if src.Type().AssignableTo(elem) {
			ptr := reflect.New(elem)
			ptr.Elem().Set(src)
			dst.Set(ptr)
			return nil
		}
		if src.Type().ConvertibleTo(elem) {
			ptr := reflect.New(elem)
			ptr.Elem().Set(src.Convert(elem))
			dst.Set(ptr)
			return nil
		}
	}
	return fmt.Errorf("cannot assign decoded %T to %s", decoded, dst.Type().String())
}
