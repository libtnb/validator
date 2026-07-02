package validator

import (
	"reflect"
	"unsafe"
)

// maxDerefDepth bounds unwrapping so a pointer cycle can't hang.
const maxDerefDepth = 32

// mapKeyOrder is a deterministic total order over map entries (map iteration is randomized).
type mapKeyOrder struct{ keyRepr, keyType, valRepr, valType string }

func (a mapKeyOrder) less(b mapKeyOrder) bool {
	if a.keyRepr != b.keyRepr {
		return a.keyRepr < b.keyRepr
	}
	if a.keyType != b.keyType {
		return a.keyType < b.keyType
	}
	if a.valRepr != b.valRepr {
		return a.valRepr < b.valRepr
	}
	return a.valType < b.valType
}

// unwrap dereferences pointer/interface chains; any typed-nil becomes plain nil.
func unwrap(v any) any {
	if v == nil {
		return nil
	}
	rv := unwrapValue(reflect.ValueOf(v))
	if !rv.IsValid() {
		return nil
	}
	return rv.Interface()
}

func unwrapValue(rv reflect.Value) reflect.Value {
	for range maxDerefDepth {
		if !rv.IsValid() {
			return reflect.Value{}
		}
		k := rv.Kind()
		if k != reflect.Pointer && k != reflect.Interface {
			break
		}
		if rv.IsNil() {
			return reflect.Value{}
		}
		rv = rv.Elem()
	}
	if rv.IsValid() {
		switch rv.Kind() {
		case reflect.Map, reflect.Slice, reflect.Chan, reflect.Func, reflect.UnsafePointer:
			if rv.IsNil() {
				return reflect.Value{}
			}
		}
	}
	return rv
}

func dynType(v reflect.Value) reflect.Type {
	if v.Kind() == reflect.Interface && !v.IsNil() {
		return v.Elem().Type()
	}
	return v.Type()
}

func valToAny(rv reflect.Value) any {
	if !rv.IsValid() || !rv.CanInterface() {
		return nil
	}
	return rv.Interface()
}

func getValueV(val reflect.Value) reflect.Value {
	if val.CanInterface() {
		return unwrapValue(val)
	}
	if val.CanAddr() {
		return unwrapValue(reflect.NewAt(val.Type(), unsafe.Pointer(val.UnsafeAddr())).Elem())
	}
	switch val.Kind() {
	case reflect.Bool:
		return reflect.ValueOf(val.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return reflect.ValueOf(val.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return reflect.ValueOf(val.Uint())
	case reflect.Float32, reflect.Float64:
		return reflect.ValueOf(val.Float())
	case reflect.Complex64, reflect.Complex128:
		return reflect.ValueOf(val.Complex())
	case reflect.String:
		return reflect.ValueOf(val.String())
	default:
		return reflect.Value{}
	}
}
