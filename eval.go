package validator

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/libtnb/validator/conv"
)

var fieldPool = sync.Pool{New: func() any { return &field{} }}

func (vd *validation) evalField(name, expr string, ctx context.Context, dst []FieldError) []FieldError {
	ds := vd.validator.splitDive(expr)
	if ds.err != nil {
		return append(dst, vd.diag(name, ds.err.Error()))
	}
	if ds.nested {
		return append(dst, vd.diag(name, "validator: multiple top-level 'dive' is not supported"))
	}
	var container, element *compiled
	if ds.container != "" {
		c, err := vd.validator.compile(ds.container)
		if err != nil {
			return append(dst, vd.diag(name, err.Error()))
		}
		container = c
	}
	if ds.hasDive && ds.element != "" {
		e, err := vd.validator.compile(ds.element)
		if err != nil {
			return append(dst, vd.diag(name, err.Error()))
		}
		element = e
	}
	return vd.evalCompiledField(compiledField{name: name, container: container, element: element, hasDive: ds.hasDive}, ctx, dst)
}

// Recover dive reflection panics; otherwise fatal under WithParallel.
func (vd *validation) evalCompiledField(cf compiledField, ctx context.Context, dst []FieldError) (out []FieldError) {
	out = dst
	defer func() {
		if r := recover(); r != nil {
			// conv.ToString, not %v: a panic value whose String() panics must not re-escape recovery
			out = append(dst, vd.diag(cf.name, "validator: field evaluation panicked: "+conv.ToString(r)))
		}
	}()
	if cf.buildErr != "" {
		return append(dst, vd.diag(cf.name, cf.buildErr))
	}
	if cf.container != nil {
		var cval reflect.Value
		if cf.hasDive {
			cval = vd.rawValue(cf)
		} else {
			cval = vd.containerValue(cf)
		}
		out = vd.evalCompiled(cf.name, cf.name, cf.container, cval, ctx, out)
	}
	if cf.hasDive && cf.element != nil {
		// dive uses the raw collection; filtering would stringify it.
		out = vd.evalDive(cf.name, cf.element, vd.rawValue(cf), ctx, out)
	}
	return out
}

func (vd *validation) rawValue(cf compiledField) reflect.Value {
	if cf.index != nil {
		return vd.valueByIndex(cf.index)
	}
	v, _ := vd.srcLookup(cf.name)
	return v
}

func (vd *validation) containerValue(cf compiledField) reflect.Value {
	if vd.filtered != nil {
		if fv, ok := vd.filtered[cf.name]; ok {
			return toValue(fv)
		}
	}
	return vd.rawValue(cf)
}

func (vd *validation) valueByIndex(index []int) reflect.Value {
	fv := vd.ssVal
	for _, idx := range index {
		sv, ok := derefToStruct(fv)
		if !ok {
			return reflect.Value{}
		}
		fv = sv.Field(idx)
	}
	return getValueV(fv)
}

func (vd *validation) diag(field, msg string) FieldError {
	return FieldError{Field: field, Message: msg}
}

// Recover rule/hook panics; otherwise fatal under WithParallel.
// name is the error key; scope is the cross-field identity (the container name
// for dive elements) so sibling resolution matches the fast path exactly.
func (vd *validation) evalCompiled(name, scope string, compiled *compiled, val reflect.Value, ctx context.Context, dst []FieldError) (out []FieldError) {
	f := fieldPool.Get().(*field)
	f.name, f.scope, f.rv, f.ctx, f.vd = name, scope, val, ctx, vd
	out = dst
	defer func() {
		*f = field{}
		fieldPool.Put(f)
		if r := recover(); r != nil {
			// conv.ToString, not %v: see evalCompiledField
			out = append(dst, vd.diag(name, "validator: rule panicked: "+conv.ToString(r)))
		}
	}()
	_, out = compiled.Diag(f, dst)
	return out
}

func (vd *validation) evalDive(name string, element *compiled, val reflect.Value, ctx context.Context, dst []FieldError) []FieldError {
	// val is pre-unwrapped; invalid Value means nil container.
	if !val.IsValid() {
		return dst
	}
	switch val.Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < val.Len(); i++ {
			// unwrap so rules see the pointee, matching every source
			ev := unwrapValue(val.Index(i))
			// fast probe first: the "field[i]" key is built only for a failing
			// element (rules must be deterministic; ErrorRules idempotent).
			if vd.exprPasses(name, element, ev, ctx) {
				continue
			}
			dst = vd.evalCompiled(name+"["+strconv.Itoa(i)+"]", name, element, ev, ctx, dst)
		}
	case reflect.Map:
		dst = vd.evalDiveMap(name, element, val, ctx, dst)
	default:
		// non-collection: fail-closed if non-empty, else skip (omitempty)
		if !IsEmptyValue(val) {
			dst = append(dst, vd.diag(name, "validator: dive requires an array or map value"))
		}
	}
	return dst
}

// MapRange, not MapIndex: a NaN key panics .Interface(). Colliding keys get a type/ordinal suffix to stay unique.
func (vd *validation) evalDiveMap(name string, element *compiled, rv reflect.Value, ctx context.Context, dst []FieldError) []FieldError {
	type mentry struct {
		mapKeyOrder
		val reflect.Value
	}
	// Fast probe each entry first: the label machinery (stringify/sort/dedup)
	// runs only over failing entries, and only those need unique error keys.
	var failed []mentry
	for it := rv.MapRange(); it.Next(); {
		k, v := it.Key(), it.Value()
		// unwrap so rules see the pointee, matching every source
		ev := unwrapValue(v)
		if vd.exprPasses(name, element, ev, ctx) {
			continue
		}
		// value may be cyclic: use cycle-bounded conv.ToString
		failed = append(failed, mentry{
			mapKeyOrder: mapKeyOrder{
				keyRepr: fmt.Sprintf("%v", k.Interface()),
				keyType: dynType(k).String(),
				valRepr: conv.ToString(v.Interface()),
				valType: dynType(v).String(),
			},
			val: ev,
		})
	}
	if len(failed) == 0 {
		return dst
	}
	formatCount := make(map[string]int, len(failed))
	for i := range failed {
		formatCount[failed[i].keyRepr]++
	}
	// total order makes suffixes iteration-order independent; only all-four-fields-equal (NaN) entries are unordered
	sort.SliceStable(failed, func(a, b int) bool { return failed[a].less(failed[b].mapKeyOrder) })
	used := make(map[string]bool, len(failed))
	for _, e := range failed {
		display := e.keyRepr
		if formatCount[e.keyRepr] > 1 {
			display = e.keyRepr + "|" + e.keyType
		}
		label := display
		for n := 1; used[label]; n++ {
			label = fmt.Sprintf("%s#%d", display, n)
		}
		used[label] = true
		dst = vd.evalCompiled(name+"["+label+"]", name, element, e.val, ctx, dst)
	}
	return dst
}

// validFast is Valid's serial short-circuiting pass/fail path; any rule/dive panic fails closed.
func (vd *validation) validFast(ctx context.Context) (ok bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	if vd.decodeErr != "" {
		return false
	}
	// Fail closed on any rule/dive panic. Valid() builds from struct tags only
	// (applyStructTags sets rules, never filters), so the fast path carries no
	// filters.
	defer func() {
		if r := recover(); r != nil {
			ok = false
		}
	}()
	if vd.rulesShared {
		var plan []compiledField
		switch {
		case vd.ssPlan != nil:
			plan = vd.ssPlan.execPlan
		case vd.srcPlan != nil:
			plan = vd.srcPlan.plan
		}
		if plan != nil {
			for i := range plan {
				if !vd.fieldPasses(plan[i], ctx) {
					return false
				}
			}
			return true
		}
	}
	// slow path (AddRules-mutated): split+compile per field on demand
	for name, expr := range vd.rules {
		if strings.TrimSpace(expr) == "" {
			continue
		}
		if !vd.fieldPasses(vd.validator.buildCompiledField(name, expr), ctx) {
			return false
		}
	}
	return true
}

func (vd *validation) fieldPasses(cf compiledField, ctx context.Context) bool {
	if cf.buildErr != "" {
		return false
	}
	if cf.container != nil {
		var cval reflect.Value
		if cf.hasDive {
			cval = vd.rawValue(cf)
		} else {
			cval = vd.containerValue(cf)
		}
		if !vd.exprPasses(cf.name, cf.container, cval, ctx) {
			return false
		}
	}
	if cf.hasDive && cf.element != nil {
		if !vd.divePasses(cf.name, cf.element, vd.rawValue(cf), ctx) {
			return false
		}
	}
	return true
}

func (vd *validation) exprPasses(name string, compiled *compiled, val reflect.Value, ctx context.Context) bool {
	f := fieldPool.Get().(*field)
	f.name, f.scope, f.rv, f.ctx, f.vd = name, name, val, ctx, vd
	ok := compiled.Fast(f)
	*f = field{}
	fieldPool.Put(f)
	return ok
}

func (vd *validation) divePasses(name string, element *compiled, val reflect.Value, ctx context.Context) bool {
	if !val.IsValid() {
		return true
	}
	switch val.Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < val.Len(); i++ {
			if !vd.exprPasses(name, element, unwrapValue(val.Index(i)), ctx) {
				return false
			}
		}
	case reflect.Map:
		for it := val.MapRange(); it.Next(); {
			if !vd.exprPasses(name, element, unwrapValue(it.Value()), ctx) {
				return false
			}
		}
	default:
		return IsEmptyValue(val)
	}
	return true
}
