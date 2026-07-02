package validator

import (
	"context"
	"reflect"
)

var _ Field = (*field)(nil)

type field struct {
	name  string
	scope string
	rv    reflect.Value
	attrs []string
	ctx   context.Context
	vd    *validation
}

func (f *field) Val() reflect.Value { return f.rv }
func (f *field) Attrs() []string    { return f.attrs }

func (f *field) WithArgs(args []string) Field {
	f.attrs = args
	return f
}

func (f *field) Name() string { return f.name }

func (f *field) RootData() any {
	if f.vd != nil {
		return f.vd.rawData()
	}
	return nil
}

func (f *field) Context() context.Context {
	if f.ctx == nil {
		return context.Background()
	}
	return f.ctx
}

// SiblingValue resolves another field's value: relative-first, then root, never self.
func (f *field) SiblingValue(name string) (reflect.Value, bool) {
	if f.vd == nil {
		return reflect.Value{}, false
	}
	return f.vd.resolveScoped(f.scope, name)
}

// scopeName is the field's stable identity for cross-field lookups: the dive
// container name (whose Name() is a bracketed error key), else the field name.
func (f *field) scopeName() string { return f.scope }

func (f *field) Sibling(name string) (Field, bool) {
	v, ok := f.SiblingValue(name)
	if !ok {
		return nil, false
	}
	return &field{name: name, scope: f.scope, rv: v, ctx: f.ctx, vd: f.vd}, true
}
