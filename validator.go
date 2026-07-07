package validator

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// Validator is a reusable, concurrency-safe factory for validations.
type Validator struct {
	registry  *registry
	exprCache *cowCache[*compiled]
	diveCache *cowCache[diveSplit]
	typeCache sync.Map
	// rules-map plan caches (see cache.go): L1 by map pointer, L2 by content, Var by expression.
	rulesPlans     sync.Map
	rulesPlanCount atomic.Int64
	contentPlans   *cowCache[*mapPlan]
	varPlans       *cowCache[*mapPlan]
	// gen counts registry changes; plan caches recheck it so a plan built concurrently with Register* is never published stale.
	gen     atomic.Uint64
	builtin bool

	tagName       string
	tagNameFunc   TagNameFunc
	transformFunc TransformFunc

	attributes             map[string]string
	messages               map[string]string
	translation            map[string]string
	translatorFn           TranslatorFunc
	strictRequired         bool
	privateFieldValidation bool
	parallel               int
}

// NewValidator creates a Validator, registering built-in rules unless WithoutBuiltinRules is given.
func NewValidator(options ...Option) *Validator {
	v := &Validator{
		registry:     newRegistry(),
		exprCache:    newCowCache[*compiled](),
		diveCache:    newCowCache[diveSplit](),
		contentPlans: newCowCache[*mapPlan](),
		varPlans:     newCowCache[*mapPlan](),
		builtin:      true,
		tagName:      "validate",
	}

	for _, o := range options {
		o(v)
	}

	if v.builtin {
		v.registerBuiltins()
	}

	return v
}

// RegisterRule registers a custom rule and invalidates caches so later validations
// pick it up; register before constructing the validations that use it. An unusable
// signature (blank, DSL syntax, reserved "dive") panics, like http.ServeMux.Handle.
func (v *Validator) RegisterRule(rule Rule) {
	validateSignature("rule", rule.Signature())
	v.registry.addRule(rule)
	v.invalidateCaches()
}

func (v *Validator) RegisterErrorRule(rule ErrorRule) {
	validateSignature("rule", rule.Signature())
	v.registry.addErrorRule(rule)
	v.invalidateCaches()
}

func (v *Validator) RegisterFilter(f Filter) {
	validateSignature("filter", f.Signature())
	v.registry.addFilter(f)
	v.invalidateCaches()
}

// RegisterFunc registers a rule from a plain function.
func (v *Validator) RegisterFunc(signature string, fn func(Field) bool, message string) {
	validateSignature("rule", signature)
	v.registry.addRule(funcRule{sig: signature, fn: fn, msg: message})
	v.invalidateCaches()
}

// RegisterStringFunc registers a string rule with the built-in conventions
// pre-applied: empty values pass (omitempty) and the value arrives rendered as
// a string, so fn only holds the actual check.
func (v *Validator) RegisterStringFunc(signature string, fn func(value string, args ...string) bool, message string) {
	v.RegisterFunc(signature, func(f Field) bool {
		rv := f.Val()
		if isEmptyV(rv) {
			return true
		}
		return fn(valString(rv), f.Attrs()...)
	}, message)
}

// CheckRules eagerly compiles every rule expression reachable from data's
// struct type (nested and embedded fields included) and reports all bad tags —
// unknown rules, DSL syntax errors, bad static args — as one joined error.
// Call it from a test or at startup to catch tag typos before request time.
func (v *Validator) CheckRules(data any) error {
	t := reflect.TypeOf(data)
	if t != nil {
		t = derefType(t)
	}
	if t == nil || t.Kind() != reflect.Struct {
		return errors.New("validator: CheckRules requires a struct or struct pointer")
	}
	var errs []error
	for _, cf := range v.getStructPlan(t).execPlan {
		if cf.buildErr != "" {
			errs = append(errs, errors.New(cf.name+": "+cf.buildErr))
		}
	}
	return errors.Join(errs...)
}

// Any validates a map, a struct (tags are read), or a scalar.
func (v *Validator) Any(data any) Validation {
	vd := newValidation(v, nil)
	v.attachSource(vd, data)
	applyStructTags(vd)
	return vd
}

// Struct validates a struct using its tags plus any added rules.
func (v *Validator) Struct(data any) Validation {
	vd := newValidation(v, nil)
	v.attachSource(vd, data)
	applyStructTags(vd)
	return vd
}

// Valid reports whether data passes its struct-tag rules on the allocation-free
// fast path; data carrying no rules is vacuously valid. Use Struct when you need
// error details, Bind, or to add rules.
func (v *Validator) Valid(data any) bool {
	vd := validationPool.Get().(*validation)
	vd.validator = v
	v.attachSource(vd, data)
	applyStructTags(vd)
	var ok bool
	if v.parallel > 0 {
		vd.errors.v = v
		vd.Validate(context.Background())
		ok = !vd.Fails()
	} else {
		ok = vd.validFast(context.Background())
	}
	vd.reset()
	validationPool.Put(vd)
	return ok
}

// Map validates a map against the given field->rule expressions.
func (v *Validator) Map(data map[string]any, rules map[string]string) Validation {
	vd := newValidation(v, mapSource{m: data})
	applyRules(vd, rules)
	return vd
}

// JSON decodes and validates a JSON object; a decode error or non-object top-level value is reported as a validation error.
func (v *Validator) JSON(data string, rules map[string]string) Validation {
	dec := json.NewDecoder(strings.NewReader(data))
	dec.UseNumber()
	var raw any
	err := dec.Decode(&raw)
	trailing := false
	if err == nil {
		// Reject trailing data, including a stray close delimiter.
		var extra json.RawMessage
		trailing = dec.Decode(&extra) != io.EOF
	}
	raw = normalizeJSONNumbers(raw)
	m, isObject := raw.(map[string]any)
	if m == nil {
		m = map[string]any{}
	}
	vd := newValidation(v, mapSource{m: m})
	applyRules(vd, rules)
	switch {
	case err != nil:
		vd.decodeErr = "validator: invalid JSON input"
	case trailing:
		vd.decodeErr = "validator: invalid JSON input"
	case !isObject:
		vd.decodeErr = "validator: JSON input is not an object"
	}
	return vd
}

// URLValues validates form data, using the first value of each key.
func (v *Validator) URLValues(data url.Values, rules map[string]string) Validation {
	m := make(map[string]any, len(data))
	for k, vs := range data {
		if len(vs) > 0 {
			m[k] = vs[0]
		} else {
			m[k] = ""
		}
	}
	vd := newValidation(v, mapSource{m: m})
	applyRules(vd, rules)
	return vd
}

// Var validates a single value; the field is named "value" and cross-field rules are not meaningful.
func (v *Validator) Var(value any, rule string) Validation {
	vd := newValidation(v, nil)
	attachVar(vd, value)
	mp := v.varPlan(rule)
	vd.rules = mp.rules
	vd.rulesShared = true
	vd.srcPlan = mp
	return vd
}

func (v *Validator) registerBuiltins() {
	for _, r := range Rules() {
		// strictRequired swaps required-family rules for their strict variant.
		if v.strictRequired {
			if sf, ok := r.(strictForm); ok {
				r = sf.withStrict()
			}
		}
		v.registry.addRule(r)
	}
	for _, r := range ErrorRules() {
		v.registry.addErrorRule(r)
	}
	for _, f := range Filters() {
		v.registry.addFilter(f)
	}
}

// invalidateCaches drops expression-derived caches after a registry change (they
// hold programs bound to the old registry). The gen bump precedes the clears so an
// in-flight plan build observes the change (see getStructPlan).
func (v *Validator) invalidateCaches() {
	v.gen.Add(1)
	v.exprCache.clear()
	v.diveCache.clear()
	v.typeCache.Clear()
	v.contentPlans.clear()
	v.varPlans.clear()
	v.rulesPlans.Clear()
	v.rulesPlanCount.Store(0)
}

type funcRule struct {
	sig string
	msg string
	fn  func(Field) bool
}

func (r funcRule) Signature() string   { return r.sig }
func (r funcRule) Passes(f Field) bool { return r.fn(f) }
func (r funcRule) Message() string     { return r.msg }

// validateSignature panics on a signature the DSL could never invoke.
func validateSignature(kind, sig string) {
	if sig == "" {
		panic("validator: " + kind + " signature must not be empty")
	}
	if sig == "dive" {
		panic(`validator: "dive" is a reserved keyword and cannot be a ` + kind + " signature")
	}
	for i := 0; i < len(sig); i++ {
		c := sig[i]
		ident := c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (i > 0 && c >= '0' && c <= '9')
		if !ident {
			panic("validator: invalid " + kind + " signature " + strconv.Quote(sig) + ": use letters, digits and '_', starting with a letter or '_'")
		}
	}
}

// attachVar stores a single value inline (no source boxing, no rules map).
func attachVar(vd *validation, value any) {
	vd.isVar = true
	if value != nil {
		vd.ssVal = reflect.ValueOf(value)
	}
}

// applyStructTags shares the type's cached rule map read-only; AddRules/RemoveRules copy-on-write before mutating.
func applyStructTags(vd *validation) {
	if vd.ssPlan != nil {
		vd.rules = vd.ssPlan.rules
		vd.rulesShared = true
	}
}

// applyRules attaches the precompiled plan for a caller rules map; the plan's rules
// snapshot is shared read-only (AddRules/RemoveRules copy-on-write), so the caller's
// map is never touched or retained.
func applyRules(vd *validation, rules map[string]string) {
	if len(rules) == 0 {
		return
	}
	mp := vd.validator.rulesPlan(rules)
	vd.rules = mp.rules
	vd.rulesShared = true
	vd.srcPlan = mp
}

func normalizeJSONNumbers(v any) any {
	switch t := v.(type) {
	case json.Number:
		s := string(t)
		if i, err := strconv.ParseInt(s, 10, 64); err == nil {
			return i
		}
		if u, err := strconv.ParseUint(s, 10, 64); err == nil {
			return u
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f
		}
		return s
	case map[string]any:
		for k, e := range t {
			t[k] = normalizeJSONNumbers(e)
		}
		return t
	case []any:
		for i, e := range t {
			t[i] = normalizeJSONNumbers(e)
		}
		return t
	default:
		return v
	}
}
