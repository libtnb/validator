package validator

import (
	"errors"
	"reflect"
	"sort"

	"github.com/libtnb/validator/conv"
)

var (
	errBindTarget       = errors.New("validator: Bind/SafeBind requires a non-nil pointer to a struct")
	errNotValidated     = errors.New("validator: call Validate before SafeBind")
	errValidationFailed = errors.New("validator: validation failed")
)

func (vd *validation) bindInto(ptr any, filtered bool) error {
	rv := reflect.ValueOf(ptr)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return errBindTarget
	}
	elem := rv.Elem()
	if elem.Kind() != reflect.Struct {
		return errBindTarget
	}
	plan := vd.validator.getStructPlan(elem.Type())
	// Bind never drops untagged fields.
	for _, fp := range plan.entries {
		if !fp.leaf {
			continue
		}
		val, ok := vd.bindValue(fp.name, filtered)
		if !ok || val == nil {
			continue
		}
		// convert into a detached temp first: a failed conversion must not leave
		// freshly allocated intermediate pointers on the target
		ft, ok := fieldTypeByIndex(elem.Type(), fp.index)
		if !ok {
			continue
		}
		tmp := reflect.New(ft).Elem()
		if !vd.setReflect(tmp, val, 0) {
			continue
		}
		fv, ok := settableField(elem, fp.index)
		if ok && fv.CanSet() {
			fv.Set(tmp)
		}
	}
	return nil
}

func (vd *validation) bindValue(name string, filtered bool) (any, bool) {
	if filtered {
		rv, ok := vd.fieldValue(name)
		return valToAny(rv), ok
	}
	rv, ok := vd.srcLookup(name)
	return valToAny(rv), ok
}

// setReflect returns true iff a value was written; a failed conversion leaves the field intact.
func (vd *validation) setReflect(fv reflect.Value, val any, depth int) bool {
	if val == nil {
		return false
	}
	if depth > maxDerefDepth {
		// bounded against recursive pointer type (type P *P).
		return false
	}
	rv := reflect.ValueOf(val)
	ft := fv.Type()
	if rv.Type().AssignableTo(ft) {
		fv.Set(rv)
		return true
	}
	if ft.Kind() == reflect.Pointer {
		// commit the pointer only if the inner conversion succeeds.
		nv := reflect.New(ft.Elem())
		if !vd.setReflect(nv.Elem(), val, depth+1) {
			return false
		}
		fv.Set(nv)
		return true
	}
	// time.Time via conv.ToTime, never a silent zero time.
	if ft == timeType {
		if t, err := conv.ToTime(val); err == nil {
			fv.Set(reflect.ValueOf(t))
			return true
		}
		return false
	}
	switch ft.Kind() {
	case reflect.String:
		fv.SetString(conv.ToString(val))
		return true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		// skip on overflow rather than write a wrapped value.
		if i, err := conv.ToInt(val); err == nil && !fv.OverflowInt(i) {
			fv.SetInt(i)
			return true
		}
		return false
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		// unsigned converter so uint64 above MaxInt64 is not rejected.
		if u, err := conv.ToUint(val); err == nil && !fv.OverflowUint(u) {
			fv.SetUint(u)
			return true
		}
		return false
	case reflect.Float32, reflect.Float64:
		if f, err := conv.ToFloat(val); err == nil && !fv.OverflowFloat(f) {
			fv.SetFloat(f)
			return true
		}
		return false
	case reflect.Bool:
		if b, err := conv.ToBool(val); err == nil {
			fv.SetBool(b)
			return true
		}
		return false
	case reflect.Struct:
		return vd.setStruct(fv, val, depth+1)
	case reflect.Slice:
		// positional convert: a failed/nil element keeps the zero, not shifting later indices.
		if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
			// non-slice source may still whole-value convert (string -> []byte).
			if rv.Type().ConvertibleTo(ft) {
				fv.Set(rv.Convert(ft))
				return true
			}
			return false
		}
		out := reflect.MakeSlice(ft, rv.Len(), rv.Len())
		for i := 0; i < rv.Len(); i++ {
			vd.setReflect(out.Index(i), unwrap(rv.Index(i).Interface()), depth+1)
		}
		fv.Set(out)
		return true
	case reflect.Array:
		if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
			return false
		}
		n := min(ft.Len(), rv.Len())
		wrote := false
		for i := range n {
			if vd.setReflect(fv.Index(i), unwrap(rv.Index(i).Interface()), depth+1) {
				wrote = true
			}
		}
		return wrote
	case reflect.Map:
		// write an entry only when both key and value convert (no phantom zero-key entry).
		if rv.Kind() != reflect.Map {
			return false
		}
		// deterministic order: source keys colliding after key conversion need a
		// stable survivor (map iteration is randomized)
		type kvPair struct {
			mapKeyOrder
			k, v reflect.Value
		}
		entries := make([]kvPair, 0, rv.Len())
		for iter := rv.MapRange(); iter.Next(); {
			k, e := iter.Key(), iter.Value()
			entries = append(entries, kvPair{
				mapKeyOrder: mapKeyOrder{
					keyRepr: conv.ToString(k.Interface()),
					keyType: dynType(k).String(),
					valRepr: conv.ToString(e.Interface()),
					valType: dynType(e).String(),
				},
				k: k, v: e,
			})
		}
		sort.Slice(entries, func(a, b int) bool { return entries[a].less(entries[b].mapKeyOrder) })
		out := reflect.MakeMapWithSize(ft, rv.Len())
		kt, et := ft.Key(), ft.Elem()
		wrote := false
		for _, en := range entries {
			kp := reflect.New(kt).Elem()
			ep := reflect.New(et).Elem()
			if vd.setReflect(kp, unwrap(en.k.Interface()), depth+1) && vd.setReflect(ep, unwrap(en.v.Interface()), depth+1) {
				out.SetMapIndex(kp, ep)
				wrote = true
			}
		}
		// empty {} succeeds; an all-fail non-empty source leaves the field untouched.
		if wrote || rv.Len() == 0 {
			fv.Set(out)
			return true
		}
		return false
	default:
		if rv.Type().ConvertibleTo(ft) {
			fv.Set(rv.Convert(ft))
			return true
		}
		return false
	}
}

func (vd *validation) setStruct(fv reflect.Value, val any, depth int) bool {
	m, ok := asStringMap(unwrap(val))
	if !ok {
		return false
	}
	plan := vd.validator.getStructPlan(fv.Type())
	wrote := false
	for _, fp := range plan.entries {
		if !fp.leaf {
			continue
		}
		ev, ok := lookupInMap(m, fp.name)
		if !ok || !ev.IsValid() {
			continue
		}
		sub, ok := settableField(fv, fp.index)
		if ok && sub.CanSet() && vd.setReflect(sub, valToAny(ev), depth+1) {
			wrote = true
		}
	}
	return wrote
}

// fieldTypeByIndex resolves a leaf's type along a plan index path (pointer
// layers pierced), without touching the value.
func fieldTypeByIndex(t reflect.Type, index []int) (reflect.Type, bool) {
	for _, i := range index {
		t = derefType(t)
		if t.Kind() != reflect.Struct || i >= t.NumField() {
			return nil, false
		}
		t = t.Field(i).Type
	}
	return t, true
}

// settableField allocates nil intermediate pointers (FieldByIndex panics on nil).
func settableField(v reflect.Value, index []int) (reflect.Value, bool) {
	for _, idx := range index {
		// bounded against recursive pointer type (type P *P).
		for d := 0; d < maxDerefDepth && v.Kind() == reflect.Pointer; d++ {
			if v.IsNil() {
				if !v.CanSet() {
					return reflect.Value{}, false
				}
				v.Set(reflect.New(v.Type().Elem()))
			}
			v = v.Elem()
		}
		if v.Kind() != reflect.Struct {
			return reflect.Value{}, false
		}
		v = v.Field(idx)
	}
	return v, true
}
