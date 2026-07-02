package validator

import (
	"maps"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/libtnb/validator/internal/dsl"
)

// maxExprCache caps entries per cache to bound memory.
const maxExprCache = 10000

// cowCache copies the map under a mutex before mutation, so reads are lock-free.
type cowCache[V any] struct {
	mu sync.Mutex
	m  atomic.Pointer[map[string]V]
}

func newCowCache[V any]() *cowCache[V] {
	c := &cowCache[V]{}
	empty := map[string]V{}
	c.m.Store(&empty)
	return c
}

func (c *cowCache[V]) load(key string) (V, bool) {
	v, ok := (*c.m.Load())[key]
	return v, ok
}

func (c *cowCache[V]) clear() {
	c.mu.Lock()
	empty := map[string]V{}
	c.m.Store(&empty)
	c.mu.Unlock()
}

// getOrBuild builds once on a miss; errors aren't cached.
func (c *cowCache[V]) getOrBuild(key string, build func() (V, error)) (V, error) {
	if v, ok := c.load(key); ok {
		return v, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if v, ok := c.load(key); ok {
		return v, nil
	}
	v, err := build()
	if err != nil {
		return v, err
	}
	old := *c.m.Load()
	var nm map[string]V
	if len(old) >= maxExprCache {
		nm = make(map[string]V, 1)
	} else {
		nm = make(map[string]V, len(old)+1)
		maps.Copy(nm, old)
	}
	nm[key] = v
	c.m.Store(&nm)
	return v, nil
}

// diveSplit partitions a field expression on a top-level dive.
type diveSplit struct {
	container, element string
	hasDive, nested    bool
	err                error
}

// mapPlan precompiles a rules-map validation (Map/JSON/URLValues/Var). rules is
// a read-only snapshot shared with validations; gen stamps the registry generation.
type mapPlan struct {
	rules map[string]string
	plan  []compiledField
	gen   uint64
}

func (mp *mapPlan) matches(rules map[string]string) bool {
	if len(rules) != len(mp.rules) {
		return false
	}
	for k, v := range rules {
		if pv, ok := mp.rules[k]; !ok || pv != v {
			return false
		}
	}
	return true
}

// rulesPlan resolves the precompiled plan for a caller rules map. L1 is keyed by
// map pointer but content-verified on every hit (a reused address or stale
// registry generation never serves the wrong plan); L2 dedups by canonical content.
func (v *Validator) rulesPlan(rules map[string]string) *mapPlan {
	key := reflect.ValueOf(rules).Pointer()
	c, hadPtr := v.rulesPlans.Load(key)
	if hadPtr {
		mp := c.(*mapPlan)
		if mp.gen == v.gen.Load() && mp.matches(rules) {
			return mp
		}
	}
	built := false
	mp, _ := v.contentPlans.getOrBuild(contentKeyOf(rules), func() (*mapPlan, error) {
		built = true
		return v.buildMapPlan(rules), nil
	})
	if hadPtr || built {
		if v.rulesPlanCount.Add(1) > maxExprCache {
			v.rulesPlans.Clear()
			v.rulesPlanCount.Store(1)
		}
		v.rulesPlans.Store(key, mp)
	}
	return mp
}

// varPlan is rulesPlan for Var's single expression, keyed by the bare string.
func (v *Validator) varPlan(rule string) *mapPlan {
	mp, _ := v.varPlans.getOrBuild(rule, func() (*mapPlan, error) {
		return v.buildMapPlan(map[string]string{varFieldName: rule}), nil
	})
	return mp
}

func (v *Validator) buildMapPlan(rules map[string]string) *mapPlan {
	mp := &mapPlan{rules: copyStrMap(rules), gen: v.gen.Load()}
	mp.plan = make([]compiledField, 0, len(rules))
	for name, expr := range rules {
		if strings.TrimSpace(expr) == "" { // no-op
			continue
		}
		mp.plan = append(mp.plan, v.buildCompiledField(name, expr))
	}
	sort.Slice(mp.plan, func(i, j int) bool { return mp.plan[i].name < mp.plan[j].name })
	return mp
}

func (v *Validator) compile(expr string) (*compiled, error) {
	return v.exprCache.getOrBuild(strings.TrimSpace(expr), func() (*compiled, error) {
		node, err := dsl.Parse(expr, v.isRawArg)
		if err != nil {
			return nil, err
		}
		return compile(node, v.resolver())
	})
}

func (v *Validator) splitDive(expr string) diveSplit {
	ds, _ := v.diveCache.getOrBuild(expr, func() (diveSplit, error) {
		container, element, hasDive, err := dsl.SplitDive(expr, v.isRawArg)
		ds := diveSplit{container: container, element: element, hasDive: hasDive, err: err}
		if hasDive {
			ds.nested = dsl.ContainsDiveToken(element, v.isRawArg)
		}
		return ds, nil
	})
	return ds
}

// resolver bridges the registry to the compiler; an ErrorRule shadows a same-signature Rule.
func (v *Validator) resolver() resolver {
	return func(sig string) (Rule, ErrorRule, bool) {
		if er, ok := v.registry.errorRule(sig); ok {
			return nil, er, true
		}
		if r, ok := v.registry.rule(sig); ok {
			return r, nil, true
		}
		return nil, nil, false
	}
}

// isRawArg reports whether name's argument is read raw; ErrorRule shadows Rule.
func (v *Validator) isRawArg(name string) bool {
	type rawArger interface{ IsRawArg() bool }
	if er, ok := v.registry.errorRule(name); ok {
		if ra, ok := er.(rawArger); ok {
			return ra.IsRawArg()
		}
	} else if r, ok := v.registry.rule(name); ok {
		if ra, ok := r.(rawArger); ok {
			return ra.IsRawArg()
		}
	}
	return dsl.DefaultIsRawArg(name)
}

// contentKeyOf canonicalizes a rules map (name-sorted, length-prefixed:
// injective, so distinct contents can never collide).
func contentKeyOf(rules map[string]string) string {
	names := make([]string, 0, len(rules))
	size := 0
	for k, v := range rules {
		names = append(names, k)
		size += len(k) + len(v) + 16
	}
	slices.Sort(names)
	var b strings.Builder
	b.Grow(size)
	for _, k := range names {
		v := rules[k]
		b.WriteString(strconv.Itoa(len(k)))
		b.WriteByte(':')
		b.WriteString(k)
		b.WriteString(strconv.Itoa(len(v)))
		b.WriteByte(':')
		b.WriteString(v)
	}
	return b.String()
}
