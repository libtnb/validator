package validator

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"io"
	"net/url"
	"reflect"
	"strconv"
	"sync"
	"sync/atomic"
)

// Validator is a reusable, concurrency-safe factory for validations.
type Validator struct {
	registry  *registry
	exprCache *cowCache[*compiled]
	diveCache *cowCache[diveSplit]
	typeCache sync.Map
	// rules-map plan caches (see cache.go): L1 by map pointer, L2 by content, Value by expression.
	rulesPlans     sync.Map
	rulesPlanCount atomic.Int64
	contentPlans   *cowCache[*mapPlan]
	varPlans       *cowCache[*mapPlan]
	builtin        bool

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

// New creates an immutable Validator.
func New(options ...Option) (*Validator, error) {
	cfg := config{builtin: true, tagName: "validate"}
	for i, option := range options {
		if option == nil {
			return nil, fmt.Errorf("%w: option %d is nil", ErrInvalidOption, i)
		}
		if err := option(&cfg); err != nil {
			return nil, err
		}
	}

	v := &Validator{
		registry:               newRegistry(),
		exprCache:              newCowCache[*compiled](),
		diveCache:              newCowCache[diveSplit](),
		contentPlans:           newCowCache[*mapPlan](),
		varPlans:               newCowCache[*mapPlan](),
		builtin:                cfg.builtin,
		tagName:                cfg.tagName,
		tagNameFunc:            cfg.tagNameFunc,
		transformFunc:          cfg.transformFunc,
		attributes:             cfg.attributes,
		messages:               cfg.messages,
		translation:            cfg.translation,
		translatorFn:           cfg.translatorFn,
		strictRequired:         cfg.strictRequired,
		privateFieldValidation: cfg.privateFieldValidation,
		parallel:               cfg.parallel,
	}
	if v.builtin {
		if err := v.registerBuiltins(); err != nil {
			return nil, err
		}
	}
	for _, rule := range cfg.rules {
		if err := v.registry.addRule(rule); err != nil {
			return nil, err
		}
	}
	for _, rule := range cfg.fallibleRules {
		if err := v.registry.addFallibleRule(rule); err != nil {
			return nil, err
		}
	}
	for _, filter := range cfg.filters {
		if err := v.registry.addFilter(filter); err != nil {
			return nil, err
		}
	}
	return v, nil
}

// MustNew is New with panic-on-error semantics.
func MustNew(options ...Option) *Validator {
	v, err := New(options...)
	if err != nil {
		panic(err)
	}
	return v
}

// Check eagerly compiles every rule expression reachable from T and reports
// all invalid tags as one joined error.
func (v *Validator) Check[T any]() error {
	t, err := validationStructType[T]("Check")
	if err != nil {
		return err
	}
	return v.CheckType(t)
}

// CheckType is the reflection-oriented form of Check for schema generators.
func (v *Validator) CheckType(t reflect.Type) error {
	if t == nil {
		return errors.New("validator: CheckType requires a struct type")
	}
	t = derefType(t)
	if t.Kind() != reflect.Struct {
		return errors.New("validator: CheckType requires a struct type")
	}
	var errs []error
	for _, cf := range v.getStructPlan(t).execPlan {
		if cf.buildErr != nil {
			errs = append(errs, &RulesError{Field: cf.name, Err: cf.buildErr})
		}
	}
	return errors.Join(errs...)
}

// Struct prepares a struct-tag validation.
func (v *Validator) Struct(data any) (*Validation, error) {
	vd := newValidation(v, nil)
	v.attachSource(vd, data)
	if vd.ssPlan == nil {
		return nil, fmt.Errorf("%w: Struct requires a struct value", ErrInvalidInput)
	}
	if err := compiledFieldsError(vd.ssPlan.execPlan); err != nil {
		return nil, err
	}
	applyStructTags(vd)
	return vd, nil
}

// MustStruct is Struct with panic-on-error semantics.
func (v *Validator) MustStruct(data any) *Validation {
	validation, err := v.Struct(data)
	if err != nil {
		panic(err)
	}
	return validation
}

// Valid reports whether data passes its struct-tag rules. Rule execution errors
// are returned separately from an invalid verdict.
func (v *Validator) Valid(ctx context.Context, data any) (bool, error) {
	if ctx == nil {
		return false, ErrNilContext
	}
	vd := validationPool.Get().(*Validation)
	vd.validator = v
	v.attachSource(vd, data)
	if vd.ssPlan == nil {
		vd.reset()
		validationPool.Put(vd)
		return false, fmt.Errorf("%w: Valid requires a struct value", ErrInvalidInput)
	}
	if err := compiledFieldsError(vd.ssPlan.execPlan); err != nil {
		vd.reset()
		validationPool.Put(vd)
		return false, err
	}
	applyStructTags(vd)
	ok, err := vd.validFast(ctx)
	vd.reset()
	validationPool.Put(vd)
	return ok, err
}

// Map prepares a Validation for any map type and explicit field expressions.
// Non-string keys are rendered deterministically.
func (v *Validator) Map[M ~map[K]V, K comparable, V any](
	data M,
	rules map[string]string,
) (*Validation, error) {
	converted, ok := asStringMap(data)
	if !ok {
		return nil, fmt.Errorf("%w: Map requires a map value", ErrInvalidInput)
	}
	vd := newValidation(v, mapSource{m: converted})
	if err := applyRules(vd, rules); err != nil {
		return nil, err
	}
	return vd, nil
}

// MustMap is Map with panic-on-error semantics.
func (v *Validator) MustMap[M ~map[K]V, K comparable, V any](data M, rules map[string]string) *Validation {
	validation, err := v.Map(data, rules)
	if err != nil {
		panic(err)
	}
	return validation
}

// JSON decodes one JSON object and prepares a Validation. It preserves integer
// precision and rejects duplicate object names and invalid UTF-8.
func (v *Validator) JSON[Bytes ~[]byte | ~string](
	data Bytes,
	rules map[string]string,
) (*Validation, error) {
	input := []byte(data)
	validSyntax := jsontext.Value(input).IsValid()
	dec := json.NewDecoder(bytes.NewReader(input))
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
	switch {
	case !validSyntax || err != nil:
		return nil, fmt.Errorf("%w: invalid JSON", ErrInvalidInput)
	case trailing:
		return nil, fmt.Errorf("%w: trailing JSON data", ErrInvalidInput)
	case !isObject:
		return nil, fmt.Errorf("%w: JSON value is not an object", ErrInvalidInput)
	}
	vd := newValidation(v, mapSource{m: m})
	if err := applyRules(vd, rules); err != nil {
		return nil, err
	}
	return vd, nil
}

// MustJSON is JSON with panic-on-error semantics.
func (v *Validator) MustJSON[Bytes ~[]byte | ~string](data Bytes, rules map[string]string) *Validation {
	validation, err := v.JSON(data, rules)
	if err != nil {
		panic(err)
	}
	return validation
}

// Values prepares a Validation from the first value of each form key.
func (v *Validator) Values(data url.Values, rules map[string]string) (*Validation, error) {
	m := make(map[string]any, len(data))
	for k, vs := range data {
		if len(vs) > 0 {
			m[k] = vs[0]
		} else {
			m[k] = ""
		}
	}
	vd := newValidation(v, mapSource{m: m})
	if err := applyRules(vd, rules); err != nil {
		return nil, err
	}
	return vd, nil
}

// MustValues is Values with panic-on-error semantics.
func (v *Validator) MustValues(data url.Values, rules map[string]string) *Validation {
	validation, err := v.Values(data, rules)
	if err != nil {
		panic(err)
	}
	return validation
}

// Value prepares a Validation for one value under the field name "value".
func (v *Validator) Value(value any, rule string) (*Validation, error) {
	vd := newValidation(v, nil)
	attachVar(vd, value)
	mp := v.varPlan(rule)
	if err := compiledFieldsError(mp.plan); err != nil {
		return nil, err
	}
	vd.rules = mp.rules
	vd.rulesShared = true
	vd.srcPlan = mp
	return vd, nil
}

// MustValue is Value with panic-on-error semantics.
func (v *Validator) MustValue(value any, rule string) *Validation {
	validation, err := v.Value(value, rule)
	if err != nil {
		panic(err)
	}
	return validation
}

type funcRule struct {
	sig string
	msg string
	fn  func(*Field) bool
}

func (r funcRule) Signature() string    { return r.sig }
func (r funcRule) Passes(f *Field) bool { return r.fn(f) }
func (r funcRule) Message() string      { return r.msg }

func (v *Validator) registerBuiltins() error {
	for _, r := range Rules() {
		// strictRequired swaps required-family rules for their strict variant.
		if v.strictRequired {
			if sf, ok := r.(strictForm); ok {
				r = sf.withStrict()
			}
		}
		if err := v.registry.addRule(r); err != nil {
			return err
		}
	}
	for _, f := range Filters() {
		if err := v.registry.addFilter(f); err != nil {
			return err
		}
	}
	return nil
}

func validationStructType[T any](operation string) (reflect.Type, error) {
	t := reflect.TypeFor[T]()
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("validator: %s requires a non-pointer struct type", operation)
	}
	return t, nil
}

func validateSignature(kind, sig string) error {
	if sig == "" {
		return fmt.Errorf("%w: %s signature is empty", ErrInvalidSignature, kind)
	}
	if sig == "dive" {
		return fmt.Errorf("%w: %q is reserved", ErrInvalidSignature, sig)
	}
	for i := 0; i < len(sig); i++ {
		c := sig[i]
		ident := c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (i > 0 && c >= '0' && c <= '9')
		if !ident {
			return fmt.Errorf("%w: invalid %s signature %s", ErrInvalidSignature, kind, strconv.Quote(sig))
		}
	}
	return nil
}

// attachVar stores a single value inline (no source boxing, no rules map).
func attachVar(vd *Validation, value any) {
	vd.isVar = true
	if value != nil {
		vd.ssVal = reflect.ValueOf(value)
	}
}

// applyStructTags shares the type's cached rule map read-only; AddRules/RemoveRules copy-on-write before mutating.
func applyStructTags(vd *Validation) {
	if vd.ssPlan != nil {
		vd.rules = vd.ssPlan.rules
		vd.rulesShared = true
	}
}

// applyRules attaches the precompiled plan for a caller rules map; the plan's rules
// snapshot is shared read-only (AddRules/RemoveRules copy-on-write), so the caller's
// map is never touched or retained.
func applyRules(vd *Validation, rules map[string]string) error {
	if len(rules) == 0 {
		return nil
	}
	mp := vd.validator.rulesPlan(rules)
	if err := compiledFieldsError(mp.plan); err != nil {
		return err
	}
	vd.rules = mp.rules
	vd.rulesShared = true
	vd.srcPlan = mp
	return nil
}

func compiledFieldsError(fields []compiledField) error {
	var errs []error
	for _, field := range fields {
		if field.buildErr != nil {
			errs = append(errs, &RulesError{Field: field.name, Err: field.buildErr})
		}
	}
	return errors.Join(errs...)
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
