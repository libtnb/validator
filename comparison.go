package validator

import (
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

var (
	stringerType = reflect.TypeFor[fmt.Stringer]()
	errorIType   = reflect.TypeFor[error]()
)

func init() {
	registerRules(
		&inRule{}, &notInRule{}, &inCiRule{},
		&eqRule{}, &neRule{}, &eqIgnoreCaseRule{}, &neIgnoreCaseRule{},
	)
}

var (
	_ Rule = (*inRule)(nil)
	_ Rule = (*notInRule)(nil)
	_ Rule = (*inCiRule)(nil)
	_ Rule = (*eqRule)(nil)
	_ Rule = (*neRule)(nil)
	_ Rule = (*eqIgnoreCaseRule)(nil)
	_ Rule = (*neIgnoreCaseRule)(nil)
)

// inRule: value must be one of the args.
type inRule struct{}

func (r *inRule) Signature() string { return "in" }

func (r *inRule) Passes(f Field) bool { return inSet(f, true) }

func (r *inRule) Message() string { return "The selected {field} is invalid." }

// notInRule: value must not be one of the args.
type notInRule struct{}

func (r *notInRule) Signature() string { return "not_in" }

func (r *notInRule) Passes(f Field) bool { return inSet(f, false) }

func (r *notInRule) Message() string { return "The selected {field} is invalid." }

// eqRule: value must equal the arg.
type eqRule struct{}

func (r *eqRule) Signature() string { return "eq" }

func (r *eqRule) Passes(f Field) bool { return eqArg(f, true, false) }

func (r *eqRule) Message() string { return "The {field} must be equal to {0}." }

// neRule: value must not equal the arg.
type neRule struct{}

func (r *neRule) Signature() string { return "ne" }

func (r *neRule) Passes(f Field) bool { return eqArg(f, false, true) }

func (r *neRule) Message() string { return "The {field} must not be equal to {0}." }

// inCiRule: value must case-insensitively equal one of the args.
type inCiRule struct{}

func (r *inCiRule) Signature() string { return "in_ci" }

func (r *inCiRule) Passes(f Field) bool {
	rv := f.Val()
	if isEmptyV(rv) {
		return true
	}
	vs := cmpString(rv)
	return slices.ContainsFunc(f.Attrs(), func(a string) bool { return strings.EqualFold(a, vs) })
}

func (r *inCiRule) Message() string { return "The selected {field} is invalid." }

// eqIgnoreCaseRule: value must case-insensitively equal the arg.
type eqIgnoreCaseRule struct{}

func (r *eqIgnoreCaseRule) Signature() string { return "eq_ignore_case" }

func (r *eqIgnoreCaseRule) Passes(f Field) bool { return eqFoldArg(f, true, false) }

func (r *eqIgnoreCaseRule) Message() string { return "The {field} must be equal to {0}." }

// neIgnoreCaseRule: value must not case-insensitively equal the arg.
type neIgnoreCaseRule struct{}

func (r *neIgnoreCaseRule) Signature() string { return "ne_ignore_case" }

func (r *neIgnoreCaseRule) Passes(f Field) bool { return eqFoldArg(f, false, true) }

func (r *neIgnoreCaseRule) Message() string { return "The {field} must not be equal to {0}." }

// cmpString renders rv byte-identically to conv.ToString: a named String()/Error()
// type renders via that method; plain builtins keep the 0-alloc kind path.
func cmpString(rv reflect.Value) string {
	if rv.IsValid() {
		if t := rv.Type(); t.PkgPath() != "" || t.Name() != t.Kind().String() {
			if t.Implements(stringerType) || t.Implements(errorIType) {
				return valString(rv)
			}
		}
		switch rv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return strconv.FormatInt(rv.Int(), 10)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			return strconv.FormatUint(rv.Uint(), 10)
		case reflect.Float32:
			return strconv.FormatFloat(rv.Float(), 'f', -1, 32)
		case reflect.Float64:
			return strconv.FormatFloat(rv.Float(), 'f', -1, 64)
		case reflect.Bool:
			return strconv.FormatBool(rv.Bool())
		}
	}
	return valString(rv)
}

// argMatches reports value-vs-arg equality: literal first, then numeric so a
// non-canonical spelling (ne:5.0) matches float 5. Integers compare exactly, so
// values above 2^53 never collide via float64 rounding.
func argMatches(rv reflect.Value, vs, arg string) bool {
	if arg == vs {
		return true
	}
	arg = strings.TrimSpace(arg)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if c, ok := cmpIntArg(rv.Int(), arg); ok {
			return c == 0
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		if c, ok := cmpUintArg(rv.Uint(), arg); ok {
			return c == 0
		}
	case reflect.Float32:
		// compare at 32-bit width: eq:0.10 must match float32(0.1)
		if n, ok := parseNumericString(arg); ok {
			return float32(n) == float32(rv.Float())
		}
	case reflect.Float64:
		if n, ok := parseNumericString(arg); ok {
			return n == rv.Float()
		}
	}
	return false
}

func inSet(f Field, want bool) bool {
	rv := f.Val()
	if isEmptyV(rv) {
		return true
	}
	vs := cmpString(rv)
	if slices.ContainsFunc(f.Attrs(), func(a string) bool { return argMatches(rv, vs, a) }) {
		return want
	}
	return !want
}

// eqArg evaluates eq/ne; no arg returns whenNoArg.
func eqArg(f Field, equal, whenNoArg bool) bool {
	rv := f.Val()
	if isEmptyV(rv) {
		return true
	}
	attrs := f.Attrs()
	if len(attrs) == 0 {
		return whenNoArg
	}
	return argMatches(rv, cmpString(rv), attrs[0]) == equal
}

// eqFoldArg is eqArg's case-insensitive form (string comparison only).
func eqFoldArg(f Field, equal, whenNoArg bool) bool {
	rv := f.Val()
	if isEmptyV(rv) {
		return true
	}
	attrs := f.Attrs()
	if len(attrs) == 0 {
		return whenNoArg
	}
	return strings.EqualFold(cmpString(rv), attrs[0]) == equal
}
