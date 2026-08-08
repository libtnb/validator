package validator

import (
	"context"
	"reflect"
)

// Field is the value and request context presented to a Rule. Fields are
// pooled and only valid during the rule call that receives them; do not retain
// a Field or any slice returned by Attrs.
type Field struct {
	name  string
	scope string
	rv    reflect.Value
	val   reflect.Value
	attrs []string
	ctx   context.Context
	vd    *Validation

	// siblings is used by direct rule tests and zero-allocation internal callers.
	// Engine-created Fields resolve siblings through vd instead.
	siblings map[string]*Field
	data     any
}

// Reflect returns the current value. An absent or nil value is invalid.
func (f *Field) Reflect() reflect.Value {
	if f.rv.IsValid() {
		return f.rv
	}
	return f.val
}

// Value returns the current value when it has type T.
func (f *Field) Value[T any]() (T, bool) { return reflectAs[T](f.Reflect()) }

// Attrs returns parsed rule arguments. The result is read-only and only valid
// during the rule call.
func (f *Field) Attrs() []string { return f.attrs }

// Name returns the field's validation name.
func (f *Field) Name() string { return f.name }

// Root returns the complete validation input when it has type T.
func (f *Field) Root[T any]() (T, bool) {
	value := f.data
	if f.vd != nil {
		value = f.vd.rawData()
	}
	typed, ok := value.(T)
	return typed, ok
}

// Context returns the context passed to Validation.Validate.
func (f *Field) Context() context.Context {
	if f.ctx == nil {
		return context.Background()
	}
	return f.ctx
}

// SiblingValue resolves another field's value: relative-first, then root, never self.
func (f *Field) SiblingValue(name string) (reflect.Value, bool) {
	if sibling, ok := f.siblings[name]; ok && sibling != nil {
		return sibling.Reflect(), true
	}
	if f.vd == nil {
		return reflect.Value{}, false
	}
	return f.vd.resolveScoped(f.scope, name)
}

// Sibling resolves another field relative to the current scope, then from the
// root, and returns it when its value has type T.
func (f *Field) Sibling[T any](name string) (T, bool) {
	v, ok := f.SiblingValue(name)
	if !ok {
		var zero T
		return zero, false
	}
	return reflectAs[T](v)
}

// scopeName is the field's stable identity for cross-field lookups: the dive
// container name (whose Name() is a bracketed error key), else the field name.
func (f *Field) scopeName() string { return f.scope }

func reflectAs[T any](value reflect.Value) (T, bool) {
	var zero T
	if !value.IsValid() || !value.CanInterface() {
		return zero, false
	}
	typed, ok := value.Interface().(T)
	return typed, ok
}
