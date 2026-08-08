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

var fieldPool = sync.Pool{New: func() any { return &Field{} }}

func (vd *Validation) evalField(ctx context.Context, name, expr string, dst []FieldError) ([]FieldError, error) {
	ds := vd.validator.splitDive(expr)
	if ds.err != nil {
		return dst, fmt.Errorf("validator: field %q: %w", name, ds.err)
	}
	if ds.nested {
		return dst, fmt.Errorf("validator: field %q: multiple top-level dive", name)
	}
	var container, element *compiled
	if ds.container != "" {
		c, err := vd.validator.compile(ds.container)
		if err != nil {
			return dst, fmt.Errorf("validator: field %q: %w", name, err)
		}
		container = c
	}
	if ds.hasDive && ds.element != "" {
		e, err := vd.validator.compile(ds.element)
		if err != nil {
			return dst, fmt.Errorf("validator: field %q: %w", name, err)
		}
		if e.sometimes {
			return dst, fmt.Errorf("validator: field %q: %s", name, errSometimesInDive)
		}
		element = e
	}
	return vd.evalCompiledField(
		ctx,
		compiledField{
			name:      name,
			container: container,
			element:   element,
			hasDive:   ds.hasDive,
		},
		dst,
	)
}

// Reflection panics become execution errors.
func (vd *Validation) evalCompiledField(
	ctx context.Context,
	cf compiledField,
	dst []FieldError,
) (out []FieldError, err error) {
	if err := context.Cause(ctx); err != nil {
		return dst, err
	}
	out = dst
	defer func() {
		if recovered := recover(); recovered != nil {
			// conv.ToString, not %v: a panic value whose String() panics must not re-escape recovery
			err = &RuleError{Field: cf.name, Err: fmt.Errorf("%w: %s", ErrRulePanic, conv.ToString(recovered))}
		}
	}()
	if cf.buildErr != nil {
		return dst, &RulesError{Field: cf.name, Err: cf.buildErr}
	}
	if vd.sometimesAbsent(cf) {
		return out, nil
	}
	if cf.container != nil {
		cval := vd.containerValue(cf)
		if cf.hasDive {
			cval = vd.rawValue(cf)
		}
		out, err = vd.evalCompiled(ctx, cf.name, cf.name, cf.container, cval, out)
		if err != nil {
			return out, err
		}
	}
	if cf.hasDive && cf.element != nil {
		// dive uses the raw collection; filtering would stringify it.
		out, err = vd.evalDive(ctx, cf.name, cf.element, vd.rawValue(cf), out)
	}
	return out, err
}

// sometimes distinguishes a missing map key from an explicit null. Struct
// inputs cannot preserve that distinction, so nil pointers count as absent.
func (vd *Validation) sometimesAbsent(cf compiledField) bool {
	if cf.container == nil || !cf.container.sometimes {
		return false
	}
	if vd.ssPlan != nil {
		return !vd.rawValue(cf).IsValid()
	}
	_, found := vd.srcLookup(cf.name)
	return !found
}

func (vd *Validation) rawValue(cf compiledField) reflect.Value {
	if cf.index != nil {
		return vd.valueByIndex(cf.index)
	}
	v, _ := vd.srcLookup(cf.name)
	return v
}

func (vd *Validation) containerValue(cf compiledField) reflect.Value {
	if vd.filtered != nil {
		if fv, ok := vd.filtered[cf.name]; ok {
			return toValue(fv)
		}
	}
	return vd.rawValue(cf)
}

func (vd *Validation) valueByIndex(index []int) reflect.Value {
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

func (vd *Validation) diag(field, msg string) FieldError {
	return FieldError{Field: field, Message: msg}
}

// Rule panics become execution errors.
// name is the error key; scope is the cross-field identity (the container name
// for dive elements) so sibling resolution matches the fast path exactly.
func (vd *Validation) evalCompiled(
	ctx context.Context,
	name string,
	scope string,
	compiled *compiled,
	val reflect.Value,
	dst []FieldError,
) (out []FieldError, err error) {
	f := fieldPool.Get().(*Field)
	f.name, f.scope, f.rv, f.ctx, f.vd = name, scope, val, ctx, vd
	out = dst
	defer func() {
		*f = Field{}
		fieldPool.Put(f)
		if recovered := recover(); recovered != nil {
			// conv.ToString, not %v: see evalCompiledField
			err = &RuleError{Field: name, Err: fmt.Errorf("%w: %s", ErrRulePanic, conv.ToString(recovered))}
		}
	}()
	_, out, err = compiled.Diag(f, dst)
	return out, err
}

func (vd *Validation) evalDive(
	ctx context.Context,
	name string,
	element *compiled,
	val reflect.Value,
	dst []FieldError,
) ([]FieldError, error) {
	// val is pre-unwrapped; invalid Value means nil container.
	if !val.IsValid() {
		return dst, nil
	}
	switch val.Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < val.Len(); i++ {
			// unwrap so rules see the pointee, matching every source
			ev := unwrapValue(val.Index(i))
			// fast probe first: the "field[i]" key is built only for a failing
			// element; custom rules must be deterministic.
			passes, err := vd.exprPasses(ctx, name, element, ev)
			if err != nil {
				return dst, err
			}
			if passes {
				continue
			}
			dst, err = vd.evalCompiled(ctx, name+"["+strconv.Itoa(i)+"]", name, element, ev, dst)
			if err != nil {
				return dst, err
			}
		}
	case reflect.Map:
		return vd.evalDiveMap(ctx, name, element, val, dst)
	default:
		// non-collection: fail-closed if non-empty, else skip (omitempty)
		if !IsEmptyValue(val) {
			dst = append(dst, vd.diag(name, "validator: dive requires an array or map value"))
		}
	}
	return dst, nil
}

// MapRange avoids MapIndex with NaN keys. Type and ordinal suffixes keep
// rendered key collisions distinct.
func (vd *Validation) evalDiveMap(
	ctx context.Context,
	name string,
	element *compiled,
	rv reflect.Value,
	dst []FieldError,
) ([]FieldError, error) {
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
		passes, err := vd.exprPasses(ctx, name, element, ev)
		if err != nil {
			return dst, err
		}
		if passes {
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
		return dst, nil
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
		var err error
		dst, err = vd.evalCompiled(ctx, name+"["+label+"]", name, element, e.val, dst)
		if err != nil {
			return dst, err
		}
	}
	return dst, nil
}

func (vd *Validation) validFast(ctx context.Context) (ok bool, err error) {
	if ctx == nil {
		return false, ErrNilContext
	}
	if err := context.Cause(ctx); err != nil {
		return false, err
	}
	// Fail closed on any rule/dive panic. Valid() builds from struct tags only
	// (applyStructTags sets rules, never filters), so the fast path carries no
	// filters.
	defer func() {
		if recovered := recover(); recovered != nil {
			ok = false
			err = &RuleError{Err: fmt.Errorf("%w: %s", ErrRulePanic, conv.ToString(recovered))}
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
				passed, err := vd.fieldPasses(ctx, plan[i])
				if err != nil || !passed {
					return false, err
				}
			}
			return true, nil
		}
	}
	// slow path (AddRules-mutated): split+compile per field on demand
	for name, expr := range vd.rules {
		if strings.TrimSpace(expr) == "" {
			continue
		}
		passed, err := vd.fieldPasses(ctx, vd.validator.buildCompiledField(name, expr))
		if err != nil || !passed {
			return false, err
		}
	}
	return true, nil
}

func (vd *Validation) fieldPasses(ctx context.Context, cf compiledField) (bool, error) {
	if cf.buildErr != nil {
		return false, &RulesError{Field: cf.name, Err: cf.buildErr}
	}
	if vd.sometimesAbsent(cf) {
		return true, nil
	}
	if cf.container != nil {
		cval := vd.containerValue(cf)
		if cf.hasDive {
			cval = vd.rawValue(cf)
		}
		passed, err := vd.exprPasses(ctx, cf.name, cf.container, cval)
		if err != nil || !passed {
			return false, err
		}
	}
	if cf.hasDive && cf.element != nil {
		passed, err := vd.divePasses(ctx, cf.name, cf.element, vd.rawValue(cf))
		if err != nil || !passed {
			return false, err
		}
	}
	return true, nil
}

func (vd *Validation) exprPasses(
	ctx context.Context,
	name string,
	compiled *compiled,
	val reflect.Value,
) (ok bool, err error) {
	f := fieldPool.Get().(*Field)
	f.name, f.scope, f.rv, f.ctx, f.vd = name, name, val, ctx, vd
	defer func() {
		*f = Field{}
		fieldPool.Put(f)
		if recovered := recover(); recovered != nil {
			ok = false
			err = &RuleError{Field: name, Err: fmt.Errorf("%w: %s", ErrRulePanic, conv.ToString(recovered))}
		}
	}()
	return compiled.Fast(f)
}

func (vd *Validation) divePasses(
	ctx context.Context,
	name string,
	element *compiled,
	val reflect.Value,
) (bool, error) {
	if !val.IsValid() {
		return true, nil
	}
	switch val.Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < val.Len(); i++ {
			passed, err := vd.exprPasses(ctx, name, element, unwrapValue(val.Index(i)))
			if err != nil || !passed {
				return false, err
			}
		}
	case reflect.Map:
		for it := val.MapRange(); it.Next(); {
			passed, err := vd.exprPasses(ctx, name, element, unwrapValue(it.Value()))
			if err != nil || !passed {
				return false, err
			}
		}
	default:
		return IsEmptyValue(val), nil
	}
	return true, nil
}
