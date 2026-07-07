package validator

import (
	"reflect"
	"slices"
	"sort"
	"strings"
	"time"
)

var timeType = reflect.TypeFor[time.Time]()

type fieldPlan struct {
	name  string
	index []int
	rules string
	leaf  bool
	dead  bool // ambiguous promotion: unreachable via Go selectors, pruned
}

// structPlan caches a struct type's reflection plan; byName indexes ALL entries
// (cross-field/binding reach untagged fields).
type structPlan struct {
	entries  []fieldPlan
	byName   map[string]int
	rules    map[string]string // field->expression; copy-on-write before mutation
	execPlan []compiledField   // name-sorted; invalidated with expr caches on Register*
	gen      uint64            // registry generation the execPlan closures bound to

	ambiguous map[string]int // build-time: name -> depth of an ambiguous collision
}

// put resolves name collisions like Go promotion: shallower shadows deeper;
// equal-depth distinct fields are ambiguous (Go rejects the selector), so
// neither is reachable under the promoted name.
func (sp *structPlan) put(fp fieldPlan) {
	d := len(fp.index)
	if ad, ok := sp.ambiguous[fp.name]; ok {
		if d >= ad {
			return
		}
		delete(sp.ambiguous, fp.name) // a shallower field is unambiguous again
	}
	if i, ok := sp.byName[fp.name]; ok {
		existing := sp.entries[i]
		switch {
		case d < len(existing.index):
			sp.entries[i] = fp
		case d == len(existing.index) && !slices.Equal(fp.index, existing.index):
			sp.ambiguous[fp.name] = d
			sp.entries[i].dead = true
		}
		return
	}
	sp.byName[fp.name] = len(sp.entries)
	sp.entries = append(sp.entries, fp)
}

// prune drops ambiguous entries and ghost subtrees — dotted paths whose
// ancestor entry resolves to a different field (a shadowed, Go-unreachable
// path) — then reindexes. Survivors go to a fresh slice: isGhost reads
// entries/byName, so compacting in place would alias data it still consults.
func (sp *structPlan) prune() {
	kept := make([]fieldPlan, 0, len(sp.entries))
	for _, e := range sp.entries {
		if e.dead || sp.isGhost(e) {
			continue
		}
		kept = append(kept, e)
	}
	sp.entries = kept
	sp.byName = make(map[string]int, len(kept))
	for i := range kept {
		sp.byName[kept[i].name] = i
	}
	sp.ambiguous = nil
}

func (sp *structPlan) isGhost(e fieldPlan) bool {
	name := e.name
	for {
		dot := strings.LastIndex(name, ".")
		if dot < 0 {
			return false
		}
		name = name[:dot]
		if i, ok := sp.byName[name]; ok {
			a := sp.entries[i]
			if a.dead {
				return true
			}
			if len(a.index) > len(e.index) || !slices.Equal(a.index, e.index[:len(a.index)]) {
				return true
			}
		}
	}
}

type compiledField struct {
	name      string    // field/error name and cross-field scope
	index     []int     // struct field path (nil on map/var path)
	container *compiled // value rules (nil if none)
	element   *compiled // per-element dive rules (nil if no dive)
	hasDive   bool
	buildErr  string // non-empty: split/compile failed, reported verbatim at eval time
}

// structSource is the reflection-backed source for a struct value (nil-pointer safe).
type structSource struct {
	val  reflect.Value
	plan *structPlan
}

func (s structSource) lookup(name string) (reflect.Value, bool) {
	i, ok := s.plan.byName[name]
	if !ok {
		return reflect.Value{}, false
	}
	fv := s.val
	for _, idx := range s.plan.entries[i].index {
		sv, ok := derefToStruct(fv)
		if !ok {
			return reflect.Value{}, false
		}
		fv = sv.Field(idx)
	}
	return getValueV(fv), true
}

func (v *Validator) getStructPlan(t reflect.Type) *structPlan {
	// A plan's execPlan closures bind the registry state at build time; the gen
	// stamp lets every reader reject a plan a concurrent Register* has obsoleted
	// (mirrors the mapPlan cache) — including one a straddling builder published
	// into the map before it could evict its own stale entry.
	if c, ok := v.typeCache.Load(t); ok {
		sp := c.(*structPlan)
		if sp.gen == v.gen.Load() {
			return sp
		}
	}
	for range 4 {
		plan := v.buildStructPlan(t)
		if plan.gen != v.gen.Load() {
			continue // straddled a Register*: rebuild against the settled registry
		}
		actual, _ := v.typeCache.LoadOrStore(t, plan)
		asp := actual.(*structPlan)
		if asp.gen == v.gen.Load() {
			return asp
		}
		v.typeCache.CompareAndDelete(t, asp) // evict a stale straddler
	}
	// Register* storm: serve an unpublished plan, correct for this call.
	return v.buildStructPlan(t)
}

func (v *Validator) buildStructPlan(t reflect.Type) *structPlan {
	// stamp gen BEFORE reading the registry: a Register* concurrent with the
	// read bumps gen past this, so the resulting plan is seen as stale on read.
	sp := &structPlan{byName: make(map[string]int), ambiguous: map[string]int{}, gen: v.gen.Load()}
	v.collectFields(t, nil, "", sp, map[reflect.Type]bool{}, false)
	sp.prune()
	for _, fp := range sp.entries {
		if fp.rules != "" {
			if sp.rules == nil {
				sp.rules = make(map[string]string, len(sp.entries))
			}
			sp.rules[fp.name] = fp.rules
		}
	}
	sp.execPlan = v.buildExecPlan(sp)
	return sp
}

// buildExecPlan precompiles every rule-bearing field, name-sorted for deterministic output.
func (v *Validator) buildExecPlan(sp *structPlan) []compiledField {
	if len(sp.rules) == 0 {
		return nil
	}
	exec := make([]compiledField, 0, len(sp.rules))
	for name, expr := range sp.rules {
		if strings.TrimSpace(expr) == "" {
			continue
		}
		cf := v.buildCompiledField(name, expr)
		if i, ok := sp.byName[name]; ok {
			cf.index = sp.entries[i].index
		}
		exec = append(exec, cf)
	}
	sort.Slice(exec, func(i, j int) bool { return exec[i].name < exec[j].name })
	return exec
}

// buildCompiledField splits one field's expression on a top-level dive and compiles each segment.
func (v *Validator) buildCompiledField(name, expr string) compiledField {
	cf := compiledField{name: name}
	ds := v.splitDive(expr)
	if ds.err != nil {
		cf.buildErr = ds.err.Error()
		return cf
	}
	if ds.nested {
		cf.buildErr = "validator: multiple top-level 'dive' is not supported"
		return cf
	}
	cf.hasDive = ds.hasDive
	if ds.container != "" {
		c, err := v.compile(ds.container)
		if err != nil {
			cf.buildErr = err.Error()
			return cf
		}
		cf.container = c
	}
	if ds.hasDive && ds.element != "" {
		e, err := v.compile(ds.element)
		if err != nil {
			cf.buildErr = err.Error()
			return cf
		}
		if e.sometimes {
			// elements always exist inside their collection: a silent no-op, so reject
			cf.buildErr = errSometimesInDive
			return cf
		}
		cf.element = e
	}
	return cf
}

// collectFields flattens a struct (dotted nested names, flat embedded promotion);
// seen guards recursion, skipRules drops a `-` subtree's validation but keeps it bindable.
func (v *Validator) collectFields(t reflect.Type, prefixIdx []int, prefixName string, sp *structPlan, seen map[reflect.Type]bool, skipRules bool) {
	if seen[t] {
		return
	}
	seen[t] = true
	defer delete(seen, t)

	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		ft := derefType(sf.Type)
		isStruct := ft.Kind() == reflect.Struct && ft != timeType
		embedded := sf.Anonymous && isStruct

		// skip unexported non-embedded fields unless private-field validation is on
		if sf.PkgPath != "" && !embedded && !v.privateFieldValidation {
			continue
		}
		rules := sf.Tag.Get(v.tagName)
		dash := rules == "-"
		if dash || skipRules {
			rules = "" // not validated, still in the plan
		}
		childSkip := skipRules || dash

		idx := append(append([]int{}, prefixIdx...), i)

		name := sf.Name
		if v.tagNameFunc != nil {
			if n := v.tagNameFunc(sf); n != "" {
				name = n
			}
		}
		fullName := name
		if prefixName != "" {
			fullName = prefixName + "." + name
		}

		if embedded {
			// The embedded field is addressable under its type name: keep it in
			// the plan (cross-field/AddRules) and honor its tag as a value rule,
			// like a named struct field. Children promote flat (no name segment).
			if sf.PkgPath == "" || v.privateFieldValidation {
				sp.put(fieldPlan{name: fullName, index: idx, rules: rules, leaf: false})
			}
			v.collectFields(ft, idx, prefixName, sp, seen, childSkip)
			continue
		}

		if isStruct {
			if seen[ft] || !v.hasCollectableField(ft, seen) {
				// recursion cut (self-referential type) or opaque struct (no
				// reachable subfields, e.g. named time types): a leaf, so Bind
				// whole-assigns it and byName resolves it.
				sp.put(fieldPlan{name: fullName, index: idx, rules: rules, leaf: true})
				continue
			}
			// always in the plan, tagged or not: cross-field rules, AddRules and
			// Bind lookups resolve the struct itself; a rule validates it as a value.
			sp.put(fieldPlan{name: fullName, index: idx, rules: rules, leaf: false})
			v.collectFields(ft, idx, fullName, sp, seen, childSkip)
		} else {
			sp.put(fieldPlan{name: fullName, index: idx, rules: rules, leaf: true})
		}
	}
}

// hasCollectableField reports whether recursing into t would reach any field
// (exported, embedded-promoted, or unexported under private-field validation);
// seen mirrors collectFields' recursion guard.
func (v *Validator) hasCollectableField(t reflect.Type, seen map[reflect.Type]bool) bool {
	if seen[t] {
		return false
	}
	seen[t] = true
	defer delete(seen, t)

	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		ft := derefType(sf.Type)
		embedded := sf.Anonymous && ft.Kind() == reflect.Struct && ft != timeType
		if sf.PkgPath != "" && !embedded && !v.privateFieldValidation {
			continue
		}
		if embedded {
			if sf.PkgPath == "" || v.privateFieldValidation {
				return true // the embedded entry itself
			}
			if v.hasCollectableField(ft, seen) {
				return true
			}
			continue
		}
		return true
	}
	return false
}

// attachSource picks the data source: struct via ssVal/ssPlan, else src; a nil
// struct/map pointer validates as that type's zero value.
func (v *Validator) attachSource(vd *validation, data any) {
	switch d := data.(type) {
	case nil:
		vd.src = mapSource{m: map[string]any{}}
		return
	case map[string]any:
		vd.src = mapSource{m: d}
		return
	}
	rv := reflect.ValueOf(data)
	// bounded against a recursive named pointer value
	for i := 0; i < maxDerefDepth && rv.Kind() == reflect.Pointer; i++ {
		if rv.IsNil() {
			// nil struct/map pointer: validate as zero value so field rules still apply
			switch et := derefType(rv.Type()); et.Kind() {
			case reflect.Struct:
				if et != timeType {
					vd.ssVal, vd.ssPlan = reflect.New(et).Elem(), v.getStructPlan(et)
					return
				}
			case reflect.Map:
				vd.src = mapSource{m: map[string]any{}}
				return
			}
			attachVar(vd, data)
			return
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.Struct:
		// unsafe read of unexported composite fields needs addressability
		if v.privateFieldValidation && !rv.CanAddr() {
			addr := reflect.New(rv.Type()).Elem()
			addr.Set(rv)
			rv = addr
		}
		vd.ssVal, vd.ssPlan = rv, v.getStructPlan(rv.Type())
		return
	case reflect.Map:
		if m, ok := asStringMap(rv.Interface()); ok {
			vd.src = mapSource{m: m}
			return
		}
	}
	attachVar(vd, data)
}

// derefType follows pointer layers to the underlying type; bounded against type P *P.
func derefType(t reflect.Type) reflect.Type {
	for i := 0; i < maxDerefDepth && t.Kind() == reflect.Pointer; i++ {
		t = t.Elem()
	}
	return t
}

// derefToStruct follows pointers to a struct (bounded against type P *P); false on
// nil/non-struct so Field never panics.
func derefToStruct(v reflect.Value) (reflect.Value, bool) {
	for d := 0; d < maxDerefDepth && v.Kind() == reflect.Pointer; d++ {
		if v.IsNil() {
			return reflect.Value{}, false
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return reflect.Value{}, false
	}
	return v, true
}
