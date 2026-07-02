package validator

import (
	"context"
	"reflect"
)

// fakeField is the shared test double for rule unit tests; do NOT redefine it per file.
type fakeField struct {
	name     string
	val      reflect.Value
	attrs    []string
	data     any
	ctx      context.Context
	siblings map[string]Field
}

func (f fakeField) Val() reflect.Value { return f.val }
func (f fakeField) Attrs() []string    { return f.attrs }
func (f fakeField) Name() string       { return f.name }
func (f fakeField) RootData() any      { return f.data }
func (f fakeField) Context() context.Context {
	if f.ctx == nil {
		return context.Background()
	}
	return f.ctx
}

func (f fakeField) Sibling(name string) (Field, bool) {
	s, ok := f.siblings[name]
	return s, ok
}
