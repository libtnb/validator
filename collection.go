package validator

import (
	"reflect"
)

func init() { registerRules(&uniqueRule{}) }

var (
	_ Rule         = (*uniqueRule)(nil)
	_ leafCompiler = (*uniqueRule)(nil)
)

// uniqueRule: a slice/array has no duplicate elements (a map no duplicate
// values). Elements compare by canonical string plus dynamic type, so 1 and "1"
// stay distinct. Non-collection non-empty values fail closed.
type uniqueRule struct{}

func (r *uniqueRule) Signature() string { return "unique" }

func (r *uniqueRule) Passes(f Field) bool {
	rv := f.Val()
	if isEmptyV(rv) {
		return true
	}
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		n := rv.Len()
		seen := make(map[uniqueKey]struct{}, n)
		for i := range n {
			k := elemKey(unwrapValue(rv.Index(i)))
			if _, dup := seen[k]; dup {
				return false
			}
			seen[k] = struct{}{}
		}
		return true
	case reflect.Map:
		seen := make(map[uniqueKey]struct{}, rv.Len())
		for it := rv.MapRange(); it.Next(); {
			k := elemKey(unwrapValue(it.Value()))
			if _, dup := seen[k]; dup {
				return false
			}
			seen[k] = struct{}{}
		}
		return true
	default:
		return false
	}
}

func (r *uniqueRule) Message() string { return "The {field} field has duplicate values." }

func (r *uniqueRule) compilePasses([]string) func(Field) bool { return r.Passes }

type uniqueKey struct {
	repr string
	typ  reflect.Type
}

func elemKey(rv reflect.Value) uniqueKey {
	if !rv.IsValid() {
		return uniqueKey{}
	}
	return uniqueKey{repr: cmpString(rv), typ: rv.Type()}
}
