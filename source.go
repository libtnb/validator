package validator

import (
	"reflect"
	"sort"
	"strings"

	"github.com/libtnb/validator/conv"
)

const varFieldName = "value"

// source is a data set for the evaluation core.
type source interface {
	lookup(name string) (reflect.Value, bool)
	raw() any
}

type mapSource struct{ m map[string]any }

func (s mapSource) lookup(name string) (reflect.Value, bool) {
	return lookupInMap(s.m, name)
}

func (s mapSource) raw() any { return s.m }

func toValue(v any) reflect.Value {
	if v == nil {
		return reflect.Value{}
	}
	return unwrapValue(reflect.ValueOf(v))
}

// lookupInMap resolves a dotted name, nested path before flat key so a decoy key can't shadow a nested field.
func lookupInMap(m map[string]any, name string) (reflect.Value, bool) {
	if v, ok := descendDotted(m, name); ok {
		return toValue(v), true
	}
	if v, ok := m[name]; ok {
		return toValue(v), true
	}
	return reflect.Value{}, false
}

func descendDotted(m map[string]any, path string) (any, bool) {
	head, rest, ok := strings.Cut(path, ".")
	if !ok {
		return nil, false
	}
	next, ok := m[head]
	if !ok {
		return nil, false
	}
	nm, ok := asStringMap(unwrap(next))
	if !ok {
		return nil, false
	}
	if v, ok := descendDotted(nm, rest); ok {
		return v, true
	}
	if v, ok := nm[rest]; ok {
		return v, true
	}
	return nil, false
}

// asStringMap returns v as a map[string]any for any map kind (keys stringified).
func asStringMap(v any) (map[string]any, bool) {
	if v == nil {
		return nil, false
	}
	if m, ok := v.(map[string]any); ok {
		return m, true
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Map {
		return nil, false
	}
	// Sort keys so a collision (int 1 vs string "1") has a deterministic survivor.
	type kv struct {
		mapKeyOrder
		v any
	}
	entries := make([]kv, 0, rv.Len())
	for it := rv.MapRange(); it.Next(); {
		k, vv := it.Key(), it.Value()
		entries = append(entries, kv{
			mapKeyOrder: mapKeyOrder{
				keyRepr: conv.ToString(k.Interface()),
				keyType: dynType(k).String(),
				valRepr: conv.ToString(vv.Interface()),
				valType: dynType(vv).String(),
			},
			v: vv.Interface(),
		})
	}
	sort.Slice(entries, func(a, b int) bool { return entries[a].less(entries[b].mapKeyOrder) })
	m := make(map[string]any, len(entries))
	for _, e := range entries {
		m[e.keyRepr] = e.v
	}
	return m, true
}
