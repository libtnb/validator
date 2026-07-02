package validator

import (
	"cmp"
	"errors"
	"math"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/libtnb/validator/conv"
)

const maxRegexCache = 10000

// regexCache holds compiled patterns; bounded since a pattern may come from data.
var (
	regexCache    sync.Map
	regexCacheLen atomic.Int64
)

// IsEmpty reports the zero/empty value for omitempty semantics.
func IsEmpty(v any) bool {
	if v == nil {
		return true
	}
	return isEmptyV(reflect.ValueOf(v))
}

// IsEmptyValue is the reflect.Value form of IsEmpty.
func IsEmptyValue(rv reflect.Value) bool { return isEmptyV(rv) }

func isEmptyV(rv reflect.Value) bool {
	if !rv.IsValid() {
		return true
	}
	if rv.Type() == timeType {
		return rv.Interface().(time.Time).IsZero()
	}
	switch rv.Kind() {
	case reflect.String, reflect.Array, reflect.Slice, reflect.Map, reflect.Chan:
		return rv.Len() == 0
	case reflect.Bool:
		return !rv.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return rv.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return rv.Float() == 0
	case reflect.Complex64, reflect.Complex128:
		return rv.Complex() == 0
	case reflect.Pointer, reflect.Interface, reflect.Func, reflect.UnsafePointer:
		return rv.IsNil()
	default:
		// Non-time struct: NOT empty, so omitempty keeps running rules.
		return false
	}
}

func numericValue(rv reflect.Value) (float64, bool) {
	if !rv.IsValid() {
		return 0, false
	}
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(rv.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return float64(rv.Uint()), true
	case reflect.Float32, reflect.Float64:
		f := rv.Float()
		if math.IsInf(f, 0) || math.IsNaN(f) {
			return 0, false
		}
		return f, true
	case reflect.String:
		return parseNumericString(rv.String())
	case reflect.Slice:
		if rv.Type().Elem().Kind() == reflect.Uint8 {
			return parseNumericString(string(rv.Bytes()))
		}
		return 0, false
	default:
		return 0, false
	}
}

func valString(rv reflect.Value) string {
	if !rv.IsValid() {
		return ""
	}
	if rv.Kind() == reflect.String {
		return rv.String()
	}
	if !rv.CanInterface() {
		return ""
	}
	return conv.ToString(rv.Interface())
}

// parseNumericString parses a strict decimal (sign, digits, optional .fraction);
// no exponent/hex/underscores, since form data is not a Go literal. Blank fails.
func parseNumericString(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if !validDecimal(s) {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
		return 0, false
	}
	return f, true
}

func validDecimal(s string) bool {
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	start := i
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == start {
		return false
	}
	if i < len(s) && s[i] == '.' {
		i++
		fstart := i
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		if i == fstart {
			return false
		}
	}
	return i == len(s)
}

// compareSize three-way compares rv's magnitude against arg. Integers compare
// exactly as int64/uint64 (no float64 rounding above 2^53); strings and []byte by
// rune count unless numericHint forces a numeric compare; other collections by
// length. ok=false (unmeasurable value or unparsable threshold) fails closed.
func compareSize(rv reflect.Value, arg string, numericHint bool) (c int, ok bool) {
	if !rv.IsValid() {
		return 0, false
	}
	arg = strings.TrimSpace(arg)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return cmpIntArg(rv.Int(), arg)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return cmpUintArg(rv.Uint(), arg)
	case reflect.Float32, reflect.Float64:
		f := rv.Float()
		if math.IsInf(f, 0) || math.IsNaN(f) {
			return 0, false
		}
		t, tok := parseNumericString(arg)
		if !tok {
			return 0, false
		}
		return cmp.Compare(f, t), true
	case reflect.String:
		if numericHint {
			return cmpNumericString(rv.String(), arg)
		}
		return cmpLenArg(utf8.RuneCountInString(rv.String()), arg)
	case reflect.Slice:
		if numericHint && rv.Type().Elem().Kind() == reflect.Uint8 {
			return cmpNumericString(string(rv.Bytes()), arg)
		}
		return cmpLenArg(rv.Len(), arg)
	case reflect.Array, reflect.Map, reflect.Chan:
		return cmpLenArg(rv.Len(), arg)
	default:
		return 0, false
	}
}

// cmpIntArg compares v against arg: exact for integer thresholds (out-of-range
// integers still order correctly by sign), float for fractional thresholds.
func cmpIntArg(v int64, arg string) (int, bool) {
	t, err := strconv.ParseInt(arg, 10, 64)
	if err == nil {
		return cmp.Compare(v, t), true
	}
	if errors.Is(err, strconv.ErrRange) {
		// syntactically an integer, beyond int64: sign decides the order
		if strings.HasPrefix(arg, "-") {
			return 1, true
		}
		return -1, true
	}
	tf, ok := parseNumericString(arg)
	if !ok {
		return 0, false
	}
	return cmp.Compare(float64(v), tf), true
}

func cmpUintArg(v uint64, arg string) (int, bool) {
	t, err := strconv.ParseUint(arg, 10, 64)
	if err == nil {
		return cmp.Compare(v, t), true
	}
	if errors.Is(err, strconv.ErrRange) {
		return -1, true // beyond uint64: v is smaller
	}
	tf, ok := parseNumericString(arg)
	if !ok {
		return 0, false
	}
	return cmp.Compare(float64(v), tf), true
}

// cmpNumericString compares s against arg exactly as int64/uint64, else float64.
func cmpNumericString(s, arg string) (int, bool) {
	s = strings.TrimSpace(s)
	if v, err := strconv.ParseInt(s, 10, 64); err == nil {
		return cmpIntArg(v, arg)
	}
	if v, err := strconv.ParseUint(s, 10, 64); err == nil {
		return cmpUintArg(v, arg)
	}
	v, ok := parseNumericString(s)
	if !ok {
		return 0, false
	}
	t, ok := parseNumericString(arg)
	if !ok {
		return 0, false
	}
	return cmp.Compare(v, t), true
}

func cmpLenArg(n int, arg string) (int, bool) {
	t, ok := parseNumericString(arg)
	if !ok {
		return 0, false
	}
	return cmp.Compare(float64(n), t), true
}

func compileRegex(pattern string) (*regexp.Regexp, error) {
	if v, ok := regexCache.Load(pattern); ok {
		switch t := v.(type) {
		case *regexp.Regexp:
			return t, nil
		case error:
			return nil, t
		}
	}
	re, err := regexp.Compile(pattern)
	// Counter is best-effort under concurrency, so the bound is approximate.
	if regexCacheLen.Load() >= maxRegexCache {
		regexCache.Clear()
		regexCacheLen.Store(0)
	}
	if err != nil {
		regexCache.Store(pattern, err)
		regexCacheLen.Add(1)
		return nil, err
	}
	regexCache.Store(pattern, re)
	regexCacheLen.Add(1)
	return re, nil
}
