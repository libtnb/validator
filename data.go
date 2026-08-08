package validator

import (
	"errors"
	"reflect"
	"sort"

	"github.com/libtnb/validator/conv"
)

var (
	// ErrBindTarget reports an invalid Bind or ValidateAs destination.
	ErrBindTarget = errors.New("validator: bind requires a non-nil pointer to a non-pointer struct")
	// ErrBindConversion reports a field conversion failure.
	ErrBindConversion = errors.New("validator: bind conversion failed")
	// ErrValidationFailed marks a ValidateAs validation failure.
	ErrValidationFailed = errors.New("validator: validation failed")
	// ErrValidated reports a mutation attempted after validation.
	ErrValidated = errors.New("validator: validation already ran")
)

// BindError identifies the field that could not be converted. Binding is
// atomic, so the destination is unchanged when this error is returned.
type BindError struct {
	Field  string
	Target reflect.Type
	Value  any
	Err    error
}

func (e *BindError) Error() string {
	return "validator: bind field " + e.Field + " to " + e.Target.String() + ": " + e.Err.Error()
}

func (e *BindError) Unwrap() error { return e.Err }

func (vd *Validation) bindInto(ptr reflect.Value, filtered bool) error {
	target := ptr.Elem()
	temp := reflect.New(target.Type()).Elem()
	temp.Set(target)
	plan := vd.validator.getStructPlan(target.Type())
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
		ft, ok := fieldTypeByIndex(temp.Type(), fp.index)
		if !ok {
			return &BindError{Field: fp.name, Target: temp.Type(), Value: val, Err: ErrBindConversion}
		}
		tmp := reflect.New(ft).Elem()
		wrote, err := vd.setReflect(tmp, val, 0)
		if err != nil || !wrote {
			if err == nil {
				err = ErrBindConversion
			}
			return &BindError{Field: fp.name, Target: ft, Value: val, Err: err}
		}
		fv, ok := settableField(temp, fp.index)
		if !ok || !fv.CanSet() {
			return &BindError{Field: fp.name, Target: ft, Value: val, Err: ErrBindConversion}
		}
		fv.Set(tmp)
	}
	target.Set(temp)
	return nil
}

func (vd *Validation) bindValue(name string, filtered bool) (any, bool) {
	if filtered {
		rv, ok := vd.fieldValue(name)
		return valToAny(rv), ok
	}
	rv, ok := vd.srcLookup(name)
	return valToAny(rv), ok
}

// setReflect reports whether a value was written and why conversion failed.
func (vd *Validation) setReflect(fv reflect.Value, val any, depth int) (bool, error) {
	if val == nil {
		switch fv.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Map, reflect.Interface, reflect.Func, reflect.Chan:
			fv.SetZero()
			return true, nil
		default:
			return false, ErrBindConversion
		}
	}
	if depth > maxDerefDepth {
		// bounded against recursive pointer type (type P *P).
		return false, ErrBindConversion
	}
	rv := reflect.ValueOf(val)
	ft := fv.Type()
	if rv.Type().AssignableTo(ft) {
		fv.Set(rv)
		return true, nil
	}
	if ft.Kind() == reflect.Pointer {
		// commit the pointer only if the inner conversion succeeds.
		nv := reflect.New(ft.Elem())
		wrote, err := vd.setReflect(nv.Elem(), val, depth+1)
		if err != nil || !wrote {
			return false, err
		}
		fv.Set(nv)
		return true, nil
	}
	// time.Time via conv.ToTime, never a silent zero time.
	if ft == timeType {
		if t, err := conv.ToTime(val); err == nil {
			fv.Set(reflect.ValueOf(t))
			return true, nil
		}
		return false, ErrBindConversion
	}
	switch ft.Kind() {
	case reflect.String:
		fv.SetString(conv.ToString(val))
		return true, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		// skip on overflow rather than write a wrapped value.
		if i, err := conv.ToInt(val); err == nil && !fv.OverflowInt(i) {
			fv.SetInt(i)
			return true, nil
		}
		return false, ErrBindConversion
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		// unsigned converter so uint64 above MaxInt64 is not rejected.
		if u, err := conv.ToUint(val); err == nil && !fv.OverflowUint(u) {
			fv.SetUint(u)
			return true, nil
		}
		return false, ErrBindConversion
	case reflect.Float32, reflect.Float64:
		if f, err := conv.ToFloat(val); err == nil && !fv.OverflowFloat(f) {
			fv.SetFloat(f)
			return true, nil
		}
		return false, ErrBindConversion
	case reflect.Bool:
		if b, err := conv.ToBool(val); err == nil {
			fv.SetBool(b)
			return true, nil
		}
		return false, ErrBindConversion
	case reflect.Struct:
		return vd.setStruct(fv, val, depth+1)
	case reflect.Slice:
		if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
			// non-slice source may still whole-value convert (string -> []byte).
			if rv.Type().ConvertibleTo(ft) {
				fv.Set(rv.Convert(ft))
				return true, nil
			}
			return false, ErrBindConversion
		}
		out := reflect.MakeSlice(ft, rv.Len(), rv.Len())
		for i := 0; i < rv.Len(); i++ {
			wrote, err := vd.setReflect(out.Index(i), unwrap(rv.Index(i).Interface()), depth+1)
			if err != nil || !wrote {
				return false, ErrBindConversion
			}
		}
		fv.Set(out)
		return true, nil
	case reflect.Array:
		if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
			return false, ErrBindConversion
		}
		n := min(ft.Len(), rv.Len())
		for i := range n {
			wrote, err := vd.setReflect(fv.Index(i), unwrap(rv.Index(i).Interface()), depth+1)
			if err != nil || !wrote {
				return false, ErrBindConversion
			}
		}
		return true, nil
	case reflect.Map:
		if rv.Kind() != reflect.Map {
			return false, ErrBindConversion
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
		for _, en := range entries {
			kp := reflect.New(kt).Elem()
			ep := reflect.New(et).Elem()
			keyWrote, keyErr := vd.setReflect(kp, unwrap(en.k.Interface()), depth+1)
			valueWrote, valueErr := vd.setReflect(ep, unwrap(en.v.Interface()), depth+1)
			if keyErr != nil || valueErr != nil || !keyWrote || !valueWrote {
				return false, ErrBindConversion
			}
			out.SetMapIndex(kp, ep)
		}
		fv.Set(out)
		return true, nil
	default:
		if rv.Type().ConvertibleTo(ft) {
			fv.Set(rv.Convert(ft))
			return true, nil
		}
		return false, ErrBindConversion
	}
}

func (vd *Validation) setStruct(fv reflect.Value, val any, depth int) (bool, error) {
	m, ok := asStringMap(unwrap(val))
	if !ok {
		return false, ErrBindConversion
	}
	plan := vd.validator.getStructPlan(fv.Type())
	for _, fp := range plan.entries {
		if !fp.leaf {
			continue
		}
		ev, ok := lookupInMap(m, fp.name)
		if !ok || !ev.IsValid() {
			continue
		}
		sub, ok := settableField(fv, fp.index)
		if !ok || !sub.CanSet() {
			return false, ErrBindConversion
		}
		wrote, err := vd.setReflect(sub, valToAny(ev), depth+1)
		if err != nil || !wrote {
			return false, ErrBindConversion
		}
	}
	return true, nil
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
