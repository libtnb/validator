// Package conv provides fast type conversions. Overflow and non-finite (NaN/Inf)
// inputs error; a float-to-integer conversion truncates toward zero.
package conv

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// maxPtrDepth bounds ToString's pointer deref so a cycle can't overflow the stack.
const maxPtrDepth = 32

// maxRenderNodes bounds the value-cycle/size check in renderable.
const maxRenderNodes = 100000

// ErrConvert is the base error for a failed conversion.
var ErrConvert = errors.New("conv: cannot convert value")

// DefaultTimeLayouts are tried in order by ToTime when no layout is given.
var DefaultTimeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05",
	"2006-01-02",
	"2006/01/02",
	"15:04:05",
}

// ToString converts v to its string representation; pointers are dereferenced (nil yields "").
func ToString(v any) string { return toString(v, 0) }

// ToInt converts v to an int64; overflow/non-finite error, floats truncate toward zero.
func ToInt(v any) (int64, error) {
	switch n := v.(type) {
	case nil:
		return 0, nil
	case int:
		return int64(n), nil
	case int8:
		return int64(n), nil
	case int16:
		return int64(n), nil
	case int32:
		return int64(n), nil
	case int64:
		return n, nil
	case uint:
		if uint64(n) > math.MaxInt64 {
			return 0, fmt.Errorf("%w: uint %d exceeds int64 range", ErrConvert, n)
		}
		return int64(n), nil
	case uint8:
		return int64(n), nil
	case uint16:
		return int64(n), nil
	case uint32:
		return int64(n), nil
	case uint64:
		if n > math.MaxInt64 {
			return 0, fmt.Errorf("%w: uint64 %d exceeds int64 range", ErrConvert, n)
		}
		return int64(n), nil
	case uintptr:
		if uint64(n) > math.MaxInt64 {
			return 0, fmt.Errorf("%w: uintptr %d exceeds int64 range", ErrConvert, n)
		}
		return int64(n), nil
	case float32:
		return floatToInt(float64(n))
	case float64:
		return floatToInt(n)
	case bool:
		if n {
			return 1, nil
		}
		return 0, nil
	case string:
		return parseInt(n)
	case []byte:
		return parseInt(string(n))
	default:
		if u, ok := underlying(v); ok {
			return ToInt(u)
		}
		return 0, fmt.Errorf("%w: %T to int", ErrConvert, v)
	}
}

// ToUint converts v to a uint64; negative/overflow/non-finite error, floats truncate toward zero.
func ToUint(v any) (uint64, error) {
	switch n := v.(type) {
	case nil:
		return 0, nil
	case uint:
		return uint64(n), nil
	case uint8:
		return uint64(n), nil
	case uint16:
		return uint64(n), nil
	case uint32:
		return uint64(n), nil
	case uint64:
		return n, nil
	case uintptr:
		return uint64(n), nil
	case int:
		return intToUint(int64(n))
	case int8:
		return intToUint(int64(n))
	case int16:
		return intToUint(int64(n))
	case int32:
		return intToUint(int64(n))
	case int64:
		return intToUint(n)
	case float32:
		return floatToUint(float64(n))
	case float64:
		return floatToUint(n)
	case bool:
		if n {
			return 1, nil
		}
		return 0, nil
	case string:
		return parseUint(n)
	case []byte:
		return parseUint(string(n))
	default:
		if u, ok := underlying(v); ok {
			return ToUint(u)
		}
		return 0, fmt.Errorf("%w: %T to uint", ErrConvert, v)
	}
}

// ToFloat converts v to a float64; non-finite results (NaN/Inf) are rejected.
func ToFloat(v any) (float64, error) {
	switch n := v.(type) {
	case nil:
		return 0, nil
	case int:
		return float64(n), nil
	case int8:
		return float64(n), nil
	case int16:
		return float64(n), nil
	case int32:
		return float64(n), nil
	case int64:
		return float64(n), nil
	case uint:
		return float64(n), nil
	case uint8:
		return float64(n), nil
	case uint16:
		return float64(n), nil
	case uint32:
		return float64(n), nil
	case uint64:
		return float64(n), nil
	case uintptr:
		return float64(n), nil
	case float32:
		return finite(float64(n))
	case float64:
		return finite(n)
	case bool:
		if n {
			return 1, nil
		}
		return 0, nil
	case string:
		return parseFloat(n)
	case []byte:
		return parseFloat(string(n))
	default:
		if u, ok := underlying(v); ok {
			return ToFloat(u)
		}
		return 0, fmt.Errorf("%w: %T to float", ErrConvert, v)
	}
}

// ToBool converts v to a bool; truthy/falsy strings are accepted, numbers are true when non-zero.
func ToBool(v any) (bool, error) {
	switch b := v.(type) {
	case nil:
		return false, nil
	case bool:
		return b, nil
	case string:
		return ParseBool(b)
	case []byte:
		return ParseBool(string(b))
	case int:
		return b != 0, nil
	case int8:
		return b != 0, nil
	case int16:
		return b != 0, nil
	case int32:
		return b != 0, nil
	case int64:
		return b != 0, nil
	case uint:
		return b != 0, nil
	case uint8:
		return b != 0, nil
	case uint16:
		return b != 0, nil
	case uint32:
		return b != 0, nil
	case uint64:
		return b != 0, nil
	case uintptr:
		return b != 0, nil
	case float32:
		return b != 0, nil
	case float64:
		return b != 0, nil
	default:
		if u, ok := underlying(v); ok {
			return ToBool(u)
		}
		return false, fmt.Errorf("%w: %T to bool", ErrConvert, v)
	}
}

// ParseBool parses the usual truthy/falsy tokens (case-insensitive, trimmed); unknown input errors.
func ParseBool(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "t", "true", "yes", "y", "on":
		return true, nil
	case "", "0", "f", "false", "no", "n", "off":
		return false, nil
	default:
		return false, fmt.Errorf("%w: %q to bool", ErrConvert, s)
	}
}

// ToTime converts v to a time.Time (integers are Unix seconds); results are UTC so all paths agree.
func ToTime(v any, layouts ...string) (time.Time, error) {
	switch t := v.(type) {
	case nil:
		return time.Time{}, nil
	case time.Time:
		return t.UTC(), nil
	case string:
		return parseTime(t, layouts)
	case []byte:
		return parseTime(string(t), layouts)
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, uintptr:
		sec, err := ToInt(v)
		if err != nil {
			return time.Time{}, err
		}
		return time.Unix(sec, 0).UTC(), nil
	default:
		if u, ok := underlying(v); ok {
			return ToTime(u, layouts...)
		}
		return time.Time{}, fmt.Errorf("%w: %T to time", ErrConvert, v)
	}
}

func isNilPointer(v any) bool {
	rv := reflect.ValueOf(v)
	return rv.Kind() == reflect.Pointer && rv.IsNil()
}

func toString(v any, depth int) string {
	switch s := v.(type) {
	case nil:
		return ""
	case string:
		return s
	case []byte:
		return string(s)
	case bool:
		return strconv.FormatBool(s)
	case int:
		return strconv.Itoa(s)
	case int8:
		return strconv.FormatInt(int64(s), 10)
	case int16:
		return strconv.FormatInt(int64(s), 10)
	case int32:
		return strconv.FormatInt(int64(s), 10)
	case int64:
		return strconv.FormatInt(s, 10)
	case uint:
		return strconv.FormatUint(uint64(s), 10)
	case uint8:
		return strconv.FormatUint(uint64(s), 10)
	case uint16:
		return strconv.FormatUint(uint64(s), 10)
	case uint32:
		return strconv.FormatUint(uint64(s), 10)
	case uint64:
		return strconv.FormatUint(s, 10)
	case float32:
		return strconv.FormatFloat(float64(s), 'f', -1, 32)
	case float64:
		return strconv.FormatFloat(s, 'f', -1, 64)
	case fmt.Stringer:
		if isNilPointer(s) { // nil *T with a deref-ing String() would panic
			return ""
		}
		return safeString(s.String)
	case error:
		if isNilPointer(s) {
			return ""
		}
		return safeString(s.Error)
	default:
		// Deref pointer chains to a bounded depth (cycle guard).
		if depth < maxPtrDepth {
			if rv := reflect.ValueOf(v); rv.Kind() == reflect.Pointer {
				if rv.IsNil() {
					return ""
				}
				return toString(rv.Elem().Interface(), depth+1)
			}
		}
		// Reduce a named scalar/[]byte to its builtin so json.RawMessage renders as text, not %v bytes.
		if u, ok := underlying(v); ok {
			return toString(u, depth+1)
		}
		// fmt misses value cycles (s[0]=s, m["k"]=m); bounded walk guards the stack.
		if rv := reflect.ValueOf(v); rv.IsValid() {
			budget := maxRenderNodes
			if !renderable(rv, &budget) {
				return "<unprintable value>"
			}
		}
		// Even fmt.Sprintf can panic (a nested panicking Stringer); recover so it can't crash the caller.
		return safeString(func() string { return fmt.Sprintf("%v", v) })
	}
}

// safeString isolates a panicking user String()/Error() so stringification can never crash the caller.
func safeString(fn func() string) (out string) {
	defer func() {
		if recover() != nil {
			out = "<unprintable value>"
		}
	}()
	return fn()
}

// renderable reports whether rv is fmt-safe (no value cycle, within budget); never calls Interface.
func renderable(rv reflect.Value, budget *int) bool {
	if *budget <= 0 {
		return false
	}
	*budget--
	switch rv.Kind() {
	case reflect.Interface:
		if !rv.IsNil() {
			return renderable(rv.Elem(), budget)
		}
	case reflect.Pointer:
		// Pierce pointers so a value cycle behind a deep pointer chain is still seen;
		// a pure pointer cycle exhausts the budget and fails closed.
		if !rv.IsNil() {
			return renderable(rv.Elem(), budget)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < rv.Len(); i++ {
			if !renderable(rv.Index(i), budget) {
				return false
			}
		}
	case reflect.Map:
		for it := rv.MapRange(); it.Next(); {
			if !renderable(it.Value(), budget) {
				return false
			}
		}
	case reflect.Struct:
		for i := 0; i < rv.NumField(); i++ {
			if !renderable(rv.Field(i), budget) {
				return false
			}
		}
	}
	return true
}

// underlying re-boxes a named scalar as its builtin kind; ok=false for plain builtins.
func underlying(v any) (any, bool) {
	rv := reflect.ValueOf(v)

	if t := rv.Type(); t.PkgPath() == "" && t.Name() == t.Kind().String() {
		return nil, false
	}
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int(), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return rv.Uint(), true
	case reflect.Float32:
		// keep the 32-bit width: formatting must match a plain float32
		return float32(rv.Float()), true
	case reflect.Float64:
		return rv.Float(), true
	case reflect.Bool:
		return rv.Bool(), true
	case reflect.String:
		return rv.String(), true
	case reflect.Slice:
		if rv.Type().Elem().Kind() == reflect.Uint8 {
			return rv.Bytes(), true
		}
	}
	return nil, false
}

func floatToInt(f float64) (int64, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) || f >= float64(math.MaxInt64) || f < float64(math.MinInt64) {
		return 0, fmt.Errorf("%w: %v out of int64 range", ErrConvert, f)
	}
	return int64(f), nil
}

func parseInt(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	i, err := strconv.ParseInt(s, 10, 64)
	if err == nil {
		return i, nil
	}
	if errors.Is(err, strconv.ErrRange) {
		return 0, fmt.Errorf("%w: %q out of int64 range", ErrConvert, s)
	}
	// accept a float literal, truncated
	if f, ferr := strconv.ParseFloat(s, 64); ferr == nil {
		return floatToInt(f)
	}
	return 0, fmt.Errorf("%w: %q to int", ErrConvert, s)
}

func intToUint(i int64) (uint64, error) {
	if i < 0 {
		return 0, fmt.Errorf("%w: negative %d to uint", ErrConvert, i)
	}
	return uint64(i), nil
}

func floatToUint(f float64) (uint64, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) || f < 0 || f >= float64(math.MaxUint64) {
		return 0, fmt.Errorf("%w: %v out of uint64 range", ErrConvert, f)
	}
	return uint64(f), nil
}

func parseUint(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	u, err := strconv.ParseUint(s, 10, 64)
	if err == nil {
		return u, nil
	}
	if errors.Is(err, strconv.ErrRange) {
		return 0, fmt.Errorf("%w: %q out of uint64 range", ErrConvert, s)
	}
	// accept a float literal, truncated
	if f, ferr := strconv.ParseFloat(s, 64); ferr == nil {
		return floatToUint(f)
	}
	return 0, fmt.Errorf("%w: %q to uint", ErrConvert, s)
}

// finite rejects NaN/±Inf on every path so numeric rules never see broken ordering.
func finite(f float64) (float64, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, fmt.Errorf("%w: %v is not finite", ErrConvert, f)
	}
	return f, nil
}

func parseFloat(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %q to float", ErrConvert, s)
	}
	return finite(f)
}

func parseTime(s string, layouts []string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	if len(layouts) == 0 {
		layouts = DefaultTimeLayouts
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("%w: %q to time", ErrConvert, s)
}
