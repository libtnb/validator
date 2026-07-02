package validator

import (
	"context"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
)

var validationPool = sync.Pool{New: func() any { return &validation{} }}

var _ Validation = (*validation)(nil)

type validation struct {
	validator *Validator
	// ssVal/ssPlan: struct source; src: map/JSON sources. A Var value lives
	// inline in ssVal (isVar).
	src        source
	ssVal      reflect.Value
	ssPlan     *structPlan
	srcPlan    *mapPlan // precompiled rules-map plan (Map/JSON/URLValues/Var)
	rules      map[string]string
	filters    map[string]string
	filtered   map[string]any
	filterErrs map[string]string // field -> failed-filter diagnostic
	errors     validationErrors
	decodeErr  string
	names      []string

	rulesShared     bool // aliases a cached plan map; copy-on-write before mutation
	filtersPrepared bool
	validated       bool
	isVar           bool
}

func newValidation(v *Validator, src source) *validation {
	vd := &validation{validator: v, src: src}
	vd.errors.v = v
	return vd
}

func (vd *validation) reset() {
	items := vd.errors.items[:0]
	bases := vd.errors.bases[:0]
	names := vd.names[:0]
	*vd = validation{}
	vd.errors.items = items
	vd.errors.bases = bases
	vd.names = names
}

// Bind writes the original (unfiltered) data into ptr; no Validate required.
func (vd *validation) Bind(ptr any) error { return vd.bindInto(ptr, false) }

// SafeBind writes the filtered data into ptr after a successful Validate.
func (vd *validation) SafeBind(ptr any) error {
	if !vd.validated {
		return errNotValidated
	}
	if vd.Fails() {
		return errValidationFailed
	}
	return vd.bindInto(ptr, true)
}

// Validate is idempotent; fields run sorted for deterministic output.
func (vd *validation) Validate(ctx context.Context) {
	if vd.validated {
		return
	}
	vd.validated = true
	if ctx == nil {
		ctx = context.Background()
	}

	if vd.decodeErr != "" {
		vd.errors.add(vd.diag("", vd.decodeErr), "")
		return
	}

	vd.prepareFilters() // serial: goroutines below only read vd.filtered
	if len(vd.filterErrs) > 0 {
		names := make([]string, 0, len(vd.filterErrs))
		for n := range vd.filterErrs {
			names = append(names, n)
		}
		slices.Sort(names)
		for _, n := range names {
			vd.errors.add(vd.diag(n, vd.filterErrs[n]), n)
		}
	}

	// Fast paths: unmodified sources run their precompiled plans.
	if vd.rulesShared {
		if vd.ssPlan != nil {
			vd.runPlan(vd.ssPlan.execPlan, ctx)
			return
		}
		if vd.srcPlan != nil {
			vd.runPlan(vd.srcPlan.plan, ctx)
			return
		}
	}

	names := vd.names[:0]
	if cap(names) < len(vd.rules) {
		names = make([]string, 0, len(vd.rules))
	}
	for name := range vd.rules {
		if strings.TrimSpace(vd.rules[name]) != "" {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	vd.names = names

	threshold := vd.validator.parallel
	if threshold > 0 && len(names) >= threshold {
		vd.validateParallel(names, ctx)
		return
	}

	for _, name := range names {
		before := len(vd.errors.items)
		vd.errors.items = vd.evalField(name, vd.rules[name], ctx, vd.errors.items)
		for j := before; j < len(vd.errors.items); j++ {
			vd.errors.bases = appendGrown(vd.errors.bases, name)
		}
	}
}

func (vd *validation) Errors() Errors { return &vd.errors }

// Err returns a literal nil on success (no typed-nil trap), else the errors.
func (vd *validation) Err() error {
	if vd.Fails() {
		return &vd.errors
	}
	return nil
}

func (vd *validation) Fails() bool { return len(vd.errors.items) > 0 }

// validateParallel: bounded workers pull fields off an atomic counter into
// disjoint per-field result slots (append-unsafe shared store), merged sorted.
func (vd *validation) validateParallel(names []string, ctx context.Context) {
	results := make([][]FieldError, len(names))
	var next atomic.Int64
	runWorkers(len(names), func() {
		for {
			i := int(next.Add(1)) - 1
			if i >= len(names) {
				return
			}
			results[i] = vd.evalField(names[i], vd.rules[names[i]], ctx, nil)
		}
	})
	for i, errs := range results {
		for _, e := range errs {
			vd.errors.add(e, names[i])
		}
	}
}

// runPlan executes the precompiled struct fast path (plan is name-sorted).
func (vd *validation) runPlan(plan []compiledField, ctx context.Context) {
	threshold := vd.validator.parallel
	if threshold > 0 && len(plan) >= threshold {
		vd.runPlanParallel(plan, ctx)
		return
	}
	for _, cf := range plan {
		before := len(vd.errors.items)
		vd.errors.items = vd.evalCompiledField(cf, ctx, vd.errors.items)
		for j := before; j < len(vd.errors.items); j++ {
			vd.errors.bases = appendGrown(vd.errors.bases, cf.name)
		}
	}
}

// runPlanParallel: bounded workers over the plan, merged in plan order.
func (vd *validation) runPlanParallel(plan []compiledField, ctx context.Context) {
	results := make([][]FieldError, len(plan))
	var next atomic.Int64
	runWorkers(len(plan), func() {
		for {
			i := int(next.Add(1)) - 1
			if i >= len(plan) {
				return
			}
			results[i] = vd.evalCompiledField(plan[i], ctx, nil)
		}
	})
	for i, errs := range results {
		for _, e := range errs {
			vd.errors.add(e, plan[i].name)
		}
	}
}

// runWorkers fans work out to min(n, GOMAXPROCS) goroutines, the caller included.
func runWorkers(n int, work func()) {
	workers := min(n, runtime.GOMAXPROCS(0))
	var wg sync.WaitGroup
	wg.Add(workers - 1)
	for w := 1; w < workers; w++ {
		go func() {
			defer wg.Done()
			work()
		}()
	}
	work()
	wg.Wait()
}
