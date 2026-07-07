package validator

import (
	"math"
	"reflect"
	"strconv"
	"strings"

	"github.com/libtnb/validator/conv"
)

func init() {
	registerRules(
		&minRule{}, &maxRule{}, &betweenRule{},
		&gtRule{}, &gteRule{}, &ltRule{}, &lteRule{},
		&lenRule{}, &sizeRule{}, &digitsRule{},
		&numericRule{}, &numberRule{}, &booleanRule{},
	)
}

var (
	_ Rule = (*minRule)(nil)
	_ Rule = (*maxRule)(nil)
	_ Rule = (*betweenRule)(nil)
	_ Rule = (*gtRule)(nil)
	_ Rule = (*gteRule)(nil)
	_ Rule = (*ltRule)(nil)
	_ Rule = (*lteRule)(nil)
	_ Rule = (*lenRule)(nil)
	_ Rule = (*sizeRule)(nil)
	_ Rule = (*digitsRule)(nil)
	_ Rule = (*numericRule)(nil)
	_ Rule = (*numberRule)(nil)
	_ Rule = (*booleanRule)(nil)

	_ leafCompiler = (*digitsRule)(nil)
)

// numericHintForm marks size rules whose string comparison switches from rune
// length to numeric value when the expression asserts numeric/number.
type numericHintForm interface{ withNumericHint() Rule }

// minRule: size >= threshold.
type minRule struct{ numeric bool }

func (r *minRule) Signature() string { return "min" }

func (r *minRule) Passes(f Field) bool {
	return sizeCompare(f, r.numeric, func(c int) bool { return c >= 0 })
}

func (r *minRule) withNumericHint() Rule { return &minRule{numeric: true} }

func (r *minRule) Message() string { return "The {field} field must be at least {0}." }

// maxRule: size <= threshold.
type maxRule struct{ numeric bool }

func (r *maxRule) Signature() string { return "max" }

func (r *maxRule) Passes(f Field) bool {
	return sizeCompare(f, r.numeric, func(c int) bool { return c <= 0 })
}

func (r *maxRule) withNumericHint() Rule { return &maxRule{numeric: true} }

func (r *maxRule) Message() string { return "The {field} field must not be greater than {0}." }

// betweenRule: size within inclusive [min,max].
type betweenRule struct{ numeric bool }

func (r *betweenRule) Signature() string { return "between" }

func (r *betweenRule) Passes(f Field) bool {
	rv := f.Val()
	if isEmptyV(rv) {
		return true
	}
	attrs := f.Attrs()
	if len(attrs) < 2 {
		return false
	}
	lo, ok := compareSize(rv, attrs[0], r.numeric)
	if !ok || lo < 0 {
		return false
	}
	hi, ok := compareSize(rv, attrs[1], r.numeric)
	return ok && hi <= 0
}

func (r *betweenRule) withNumericHint() Rule { return &betweenRule{numeric: true} }

func (r *betweenRule) Message() string { return "The {field} field must be between {0} and {1}." }

// gtRule: size > threshold.
type gtRule struct{ numeric bool }

func (r *gtRule) Signature() string { return "gt" }

func (r *gtRule) Passes(f Field) bool {
	return sizeCompare(f, r.numeric, func(c int) bool { return c > 0 })
}

func (r *gtRule) withNumericHint() Rule { return &gtRule{numeric: true} }

func (r *gtRule) Message() string { return "The {field} field must be greater than {0}." }

// gteRule: size >= threshold.
type gteRule struct{ numeric bool }

func (r *gteRule) Signature() string { return "gte" }

func (r *gteRule) Passes(f Field) bool {
	return sizeCompare(f, r.numeric, func(c int) bool { return c >= 0 })
}

func (r *gteRule) withNumericHint() Rule { return &gteRule{numeric: true} }

func (r *gteRule) Message() string { return "The {field} field must be greater than or equal to {0}." }

// ltRule: size < threshold.
type ltRule struct{ numeric bool }

func (r *ltRule) Signature() string { return "lt" }

func (r *ltRule) Passes(f Field) bool {
	return sizeCompare(f, r.numeric, func(c int) bool { return c < 0 })
}

func (r *ltRule) withNumericHint() Rule { return &ltRule{numeric: true} }

func (r *ltRule) Message() string { return "The {field} field must be less than {0}." }

// lteRule: size <= threshold.
type lteRule struct{ numeric bool }

func (r *lteRule) Signature() string { return "lte" }

func (r *lteRule) Passes(f Field) bool {
	return sizeCompare(f, r.numeric, func(c int) bool { return c <= 0 })
}

func (r *lteRule) withNumericHint() Rule { return &lteRule{numeric: true} }

func (r *lteRule) Message() string { return "The {field} field must be less than or equal to {0}." }

// lenRule: size == given size.
type lenRule struct{ numeric bool }

func (r *lenRule) Signature() string { return "len" }

func (r *lenRule) Passes(f Field) bool {
	return sizeCompare(f, r.numeric, func(c int) bool { return c == 0 })
}

func (r *lenRule) withNumericHint() Rule { return &lenRule{numeric: true} }

func (r *lenRule) Message() string { return "The {field} field must be {0}." }

// sizeRule: alias of len.
type sizeRule struct{ numeric bool }

func (r *sizeRule) Signature() string { return "size" }

func (r *sizeRule) Passes(f Field) bool {
	return sizeCompare(f, r.numeric, func(c int) bool { return c == 0 })
}

func (r *sizeRule) withNumericHint() Rule { return &sizeRule{numeric: true} }

func (r *sizeRule) Message() string { return "The {field} field must be {0}." }

// digitsRule: all digits with exactly the given length.
type digitsRule struct{}

func (r *digitsRule) Signature() string { return "digits" }

func (r *digitsRule) Passes(f Field) bool {
	attrs := f.Attrs()
	if len(attrs) < 1 {
		return isEmptyV(f.Val())
	}
	want, err := strconv.ParseInt(strings.TrimSpace(attrs[0]), 10, 64)
	if err != nil {
		return isEmptyV(f.Val())
	}
	return digitsCheck(f, want)
}

// compilePasses pre-parses the length so evaluation never re-parses the arg.
func (r *digitsRule) compilePasses(args []string) func(Field) bool {
	if len(args) < 1 {
		return nil // fall back to the generic path (fails closed on non-empty)
	}
	want, err := strconv.ParseInt(strings.TrimSpace(args[0]), 10, 64)
	if err != nil {
		return nil
	}
	return func(f Field) bool { return digitsCheck(f, want) }
}

func (r *digitsRule) Message() string { return "The {field} field must be {0} digits." }

func digitsCheck(f Field, want int64) bool {
	rv := f.Val()
	if isEmptyV(rv) {
		return true
	}
	s := valString(rv)
	if int64(len(s)) != want {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// numericRule: value is a number or numeric string.
type numericRule struct{}

func (r *numericRule) Signature() string { return "numeric" }

func (r *numericRule) Passes(f Field) bool {
	if isEmptyV(f.Val()) {
		return true
	}
	_, ok := numericValue(f.Val())
	return ok
}

func (r *numericRule) Message() string { return "The {field} field must be a number." }

// numberRule: value is an integer.
type numberRule struct{}

func (r *numberRule) Signature() string { return "number" }

func (r *numberRule) Passes(f Field) bool {
	rv := f.Val()
	if isEmptyV(rv) {
		return true
	}

	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return true
	case reflect.Float32, reflect.Float64:
		x := rv.Float()
		return !math.IsInf(x, 0) && !math.IsNaN(x) && x == math.Trunc(x)
	case reflect.String:
		return intLikeString(rv.String())
	case reflect.Slice:
		if rv.Type().Elem().Kind() == reflect.Uint8 {
			return intLikeString(string(rv.Bytes()))
		}
		return false
	default:
		return false
	}
}

func (r *numberRule) Message() string { return "The {field} field must be an integer." }

// booleanRule: value is convertible to a boolean.
type booleanRule struct{}

func (r *booleanRule) Signature() string { return "boolean" }

func (r *booleanRule) Passes(f Field) bool {
	rv := f.Val()
	if isEmptyV(rv) {
		return true
	}

	switch {
	case rv.Kind() == reflect.Bool:
		return true
	case rv.Kind() == reflect.String:
		return boolLikeString(rv.String())
	case rv.Kind() == reflect.Slice && rv.Type().Elem().Kind() == reflect.Uint8:
		return boolLikeString(string(rv.Bytes()))
	default:
		if n, ok := numericValue(rv); ok {
			return n == 0 || n == 1
		}
		return false
	}
}

func (r *booleanRule) Message() string { return "The {field} field must be true or false." }

// intLikeString reports whether s is an integer string (uint64-range positives count).
func intLikeString(s string) bool {
	s = strings.TrimSpace(s)
	if _, err := strconv.ParseInt(s, 10, 64); err == nil {
		return true
	}
	_, err := strconv.ParseUint(s, 10, 64)
	return err == nil
}

// boolLikeString rejects whitespace-only input: blank is not a boolean.
func boolLikeString(s string) bool {
	if strings.TrimSpace(s) == "" {
		return false
	}
	_, err := conv.ParseBool(s)
	return err == nil
}

// sizeCompare: single-threshold size family over compareSize's three-way
// result; fails closed on a bad threshold or unmeasurable value.
func sizeCompare(f Field, numericHint bool, ok func(c int) bool) bool {
	rv := f.Val()
	if isEmptyV(rv) {
		return true
	}
	attrs := f.Attrs()
	if len(attrs) < 1 {
		return false
	}
	c, valid := compareSize(rv, attrs[0], numericHint)
	if !valid {
		return false
	}
	return ok(c)
}
