package validator

import (
	"context"
	"errors"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
)

var validationPool = sync.Pool{New: func() any { return &Validation{} }}

// Validation is one mutable validation configuration and its eventual result.
// It is not safe for concurrent use. Configure it before Validate; mutations
// after validation return ErrValidated.
type Validation struct {
	validator *Validator
	// ssVal/ssPlan: struct source; src: map/JSON sources. A Value input lives
	// inline in ssVal (isVar).
	src        source
	ssVal      reflect.Value
	ssPlan     *structPlan
	srcPlan    *mapPlan // precompiled rules-map plan (Map/JSON/Values/Value)
	rules      map[string]string
	filters    map[string]string
	filtered   map[string]any
	filterErrs map[string]string // field -> failed-filter diagnostic
	errors     Errors
	runtimeErr error
	names      []string

	rulesShared     bool // aliases a cached plan map; copy-on-write before mutation
	filtersPrepared bool
	validated       bool
	isVar           bool
}

// Bind atomically writes the original input into dst. T must be a non-pointer
// struct; on any conversion failure dst is left unchanged.
func (vd *Validation) Bind[T any](dst *T) error {
	if _, err := validationStructType[T]("Bind"); err != nil || dst == nil {
		return ErrBindTarget
	}
	return vd.bindInto(reflect.ValueOf(dst), false)
}

// ValidateAs validates and atomically writes filtered input into dst. It wraps
// ErrValidationFailed on validation errors and never partially mutates dst.
func (vd *Validation) ValidateAs[T any](ctx context.Context, dst *T) error {
	if _, err := validationStructType[T]("ValidateAs"); err != nil || dst == nil {
		return ErrBindTarget
	}
	if err := vd.Validate(ctx); err != nil {
		if vd.runtimeErr != nil {
			return err
		}
		return errors.Join(ErrValidationFailed, err)
	}
	return vd.bindInto(reflect.ValueOf(dst), true)
}

// Validate freezes the Validation on its first non-nil-context call and caches
// that result, including cancellation. Fields run in sorted order.
func (vd *Validation) Validate(ctx context.Context) error {
	if ctx == nil {
		return ErrNilContext
	}
	if vd.validated {
		return vd.Err()
	}
	vd.validated = true
	if err := context.Cause(ctx); err != nil {
		vd.runtimeErr = err
		return err
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
			vd.runPlan(ctx, vd.ssPlan.execPlan)
			return vd.Err()
		}
		if vd.srcPlan != nil {
			vd.runPlan(ctx, vd.srcPlan.plan)
			return vd.Err()
		}
	}

	names := vd.names[:0]
	if cap(names) < len(vd.rules) {
		names = make([]string, 0, len(vd.rules))
	}
	for name, expr := range vd.rules {
		if strings.TrimSpace(expr) != "" {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	vd.names = names

	threshold := vd.validator.parallel
	if threshold > 0 && len(names) >= threshold {
		vd.validateParallel(ctx, names)
		return vd.Err()
	}

	for _, name := range names {
		before := len(vd.errors.items)
		var err error
		vd.errors.items, err = vd.evalField(ctx, name, vd.rules[name], vd.errors.items)
		if err != nil {
			vd.runtimeErr = errors.Join(vd.runtimeErr, err)
		}
		for j := before; j < len(vd.errors.items); j++ {
			vd.errors.bases = appendGrown(vd.errors.bases, name)
		}
	}
	return vd.Err()
}

// Errors returns the validation's read-only error collection.
func (vd *Validation) Errors() *Errors { return &vd.errors }

// Err returns a literal nil on success (no typed-nil trap), else the errors.
func (vd *Validation) Err() error {
	var validationErr error
	if len(vd.errors.items) > 0 {
		validationErr = &vd.errors
	}
	return errors.Join(vd.runtimeErr, validationErr)
}

// Fails reports whether validation failed or could not be completed.
func (vd *Validation) Fails() bool {
	return vd.runtimeErr != nil || len(vd.errors.items) > 0
}

func (vd *Validation) reset() {
	items := vd.errors.items[:0]
	bases := vd.errors.bases[:0]
	names := vd.names[:0]
	*vd = Validation{}
	vd.errors.items = items
	vd.errors.bases = bases
	vd.names = names
}

func (vd *Validation) ensureMutable() error {
	if vd.validated {
		return ErrValidated
	}
	return nil
}

// validateParallel: bounded workers pull fields off an atomic counter into
// disjoint per-field result slots (append-unsafe shared store), merged sorted.
func (vd *Validation) validateParallel(ctx context.Context, names []string) {
	type result struct {
		fields []FieldError
		err    error
	}
	results := make([]result, len(names))
	var next atomic.Int64
	runWorkers(len(names), func() {
		for {
			i := int(next.Add(1)) - 1
			if i >= len(names) {
				return
			}
			results[i].fields, results[i].err = vd.evalField(ctx, names[i], vd.rules[names[i]], nil)
		}
	})
	for i, result := range results {
		vd.runtimeErr = errors.Join(vd.runtimeErr, result.err)
		for _, e := range result.fields {
			vd.errors.add(e, names[i])
		}
	}
}

func (vd *Validation) runPlan(ctx context.Context, plan []compiledField) {
	threshold := vd.validator.parallel
	if threshold > 0 && len(plan) >= threshold {
		vd.runPlanParallel(ctx, plan)
		return
	}
	for _, cf := range plan {
		before := len(vd.errors.items)
		var err error
		vd.errors.items, err = vd.evalCompiledField(ctx, cf, vd.errors.items)
		vd.runtimeErr = errors.Join(vd.runtimeErr, err)
		for j := before; j < len(vd.errors.items); j++ {
			vd.errors.bases = appendGrown(vd.errors.bases, cf.name)
		}
	}
}

func (vd *Validation) runPlanParallel(ctx context.Context, plan []compiledField) {
	type result struct {
		fields []FieldError
		err    error
	}
	results := make([]result, len(plan))
	var next atomic.Int64
	runWorkers(len(plan), func() {
		for {
			i := int(next.Add(1)) - 1
			if i >= len(plan) {
				return
			}
			results[i].fields, results[i].err = vd.evalCompiledField(ctx, plan[i], nil)
		}
	})
	for i, result := range results {
		vd.runtimeErr = errors.Join(vd.runtimeErr, result.err)
		for _, e := range result.fields {
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

func newValidation(v *Validator, src source) *Validation {
	vd := &Validation{validator: v, src: src}
	vd.errors.v = v
	return vd
}
