package conv

import (
	"encoding/json"
	"math"
	"reflect"
	"strconv"
	"testing"
	"time"
)

func TestToString(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{nil, ""},
		{"hi", "hi"},
		{[]byte("b"), "b"},
		{true, "true"},
		{42, "42"},
		{int64(-7), "-7"},
		{uint(8), "8"},
		{3.5, "3.5"},
		{float32(1.5), "1.5"},
	}
	for _, c := range cases {
		if got := ToString(c.in); got != c.want {
			t.Errorf("ToString(%v)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestToStringPointer(t *testing.T) {
	n := 42
	if s := ToString(&n); s != "42" {
		t.Errorf("ToString(*int) should deref, got %q", s)
	}
	var np *int
	if s := ToString(np); s != "" {
		t.Errorf("ToString(nil *int) should be empty, got %q", s)
	}
}

// ToString bounds deref depth so a pointer cycle can't overflow the stack.
func TestToStringPointerCycleBounded(t *testing.T) {
	var p any
	p = &p
	done := make(chan string, 1)
	go func() { done <- ToString(p) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ToString did not terminate on a pointer cycle")
	}

	n := 7
	pn := &n
	ppn := &pn
	if s := ToString(ppn); s != "7" {
		t.Errorf("ToString(**int) = %q, want \"7\"", s)
	}
}

// Named []byte (e.g. json.RawMessage) renders as text, not the %v byte form, to
// avoid false rule failures and a notblank bypass.
func TestToStringNamedBytes(t *testing.T) {
	type Code []byte
	if got := ToString(Code("hi")); got != "hi" {
		t.Errorf("ToString(named []byte) = %q, want %q", got, "hi")
	}
	if got := ToString(json.RawMessage(`{"a":1}`)); got != `{"a":1}` {
		t.Errorf("ToString(json.RawMessage) = %q, want %q", got, `{"a":1}`)
	}
	if got := ToString(Code("   ")); got != "   " {
		t.Errorf("ToString(blank named []byte) = %q, want three spaces", got)
	}
	type Name string
	type Age int
	if got := ToString(Name("alice")); got != "alice" {
		t.Errorf("ToString(named string) = %q", got)
	}
	if got := ToString(Age(42)); got != "42" {
		t.Errorf("ToString(named int) = %q, want 42", got)
	}
}

// fmt does not detect slice/map value cycles, so the bounded renderable guard
// renders a placeholder rather than overflowing the stack.
func TestToStringCyclicContainerNoCrash(t *testing.T) {
	s := make([]any, 1)
	s[0] = s
	if got := ToString(s); got == "" {
		t.Error("a cyclic slice should render as a non-empty placeholder, not crash")
	}

	m := map[string]any{}
	m["self"] = m
	if got := ToString(m); got == "" {
		t.Error("a cyclic map should render as a non-empty placeholder, not crash")
	}

	// high fan-out cycle must also bail in bounded (not exponential) time
	wide := make([]any, 64)
	for i := range wide {
		wide[i] = wide
	}
	_ = ToString(wide)

	if got := ToString([]int{1, 2, 3}); got != "[1 2 3]" {
		t.Errorf("normal slice render changed: %q", got)
	}
	if got := ToString(map[string]int{"a": 1}); got != "map[a:1]" {
		t.Errorf("normal map render changed: %q", got)
	}
	type pt struct{ X, Y int }
	if got := ToString(pt{1, 2}); got != "{1 2}" {
		t.Errorf("normal struct render changed: %q", got)
	}
}

func TestToStringPanickingStringer(t *testing.T) {
	// A user String()/Error() that panics must degrade to the placeholder:
	// engine-side stringification can never crash the caller.
	if got := ToString(panicStringer{}); got != "<unprintable value>" {
		t.Errorf("panicking Stringer: got %q, want placeholder", got)
	}
	if got := ToString(panicError{}); got != "<unprintable value>" {
		t.Errorf("panicking error: got %q, want placeholder", got)
	}
	// A nil-field deref inside String() is the realistic form of the same bug.
	if got := ToString(derefStringer{}); got != "<unprintable value>" {
		t.Errorf("nil-deref Stringer: got %q, want placeholder", got)
	}
}

type panicStringer struct{}

func (panicStringer) String() string { panic("stringer boom") }

type panicError struct{}

func (panicError) Error() string { panic("error boom") }

type derefStringer struct{ p *int }

func (d derefStringer) String() string { return strconv.Itoa(*d.p) }

func TestToStringValueCycleBehindDeepPointers(t *testing.T) {
	// A value-cyclic container hidden behind more pointer layers than the deref
	// bound must still hit the cycle guard, not overflow the stack in fmt.
	cyc := make([]any, 1)
	cyc[0] = cyc
	v := reflect.ValueOf(cyc)
	for range 40 {
		pv := reflect.New(v.Type())
		pv.Elem().Set(v)
		v = pv
	}
	if got := ToString(v.Interface()); got != "<unprintable value>" {
		t.Errorf("deep-pointer cycle: got %q, want placeholder", got)
	}

	// A deep pointer chain to a normal value still renders (as before).
	n := reflect.ValueOf(5)
	for range 40 {
		pv := reflect.New(n.Type())
		pv.Elem().Set(n)
		n = pv
	}
	if got := ToString(n.Interface()); got == "<unprintable value>" || got == "" {
		t.Errorf("deep pointer to int should still render, got %q", got)
	}
}

func TestToInt(t *testing.T) {
	cases := []struct {
		in   any
		want int64
		ok   bool
	}{
		{nil, 0, true},
		{5, 5, true},
		{"12", 12, true},
		{"12.9", 12, true},
		{true, 1, true},
		{3.9, 3, true},
		{"abc", 0, false},
	}
	for _, c := range cases {
		got, err := ToInt(c.in)
		if (err == nil) != c.ok {
			t.Errorf("ToInt(%v) err=%v want ok=%v", c.in, err, c.ok)
			continue
		}
		if c.ok && got != c.want {
			t.Errorf("ToInt(%v)=%d want %d", c.in, got, c.want)
		}
	}
}

func TestToIntOverflowAndNonFinite(t *testing.T) {
	for _, c := range []any{
		uint64(math.MaxUint64),
		"99999999999999999999999",
		math.Inf(1),
		math.NaN(),
		1e300,
	} {
		if got, err := ToInt(c); err == nil {
			t.Errorf("ToInt(%v) should error (overflow/non-finite), got %d", c, got)
		}
	}
	if i, err := ToInt(uint64(42)); err != nil || i != 42 {
		t.Errorf("ToInt(uint64 42) = %d, %v", i, err)
	}
	if i, err := ToInt("12.0"); err != nil || i != 12 {
		t.Errorf(`ToInt("12.0") = %d, %v`, i, err)
	}
}

// ToUint preserves a valid uint64 above math.MaxInt64 that the signed ToInt loses.
func TestToUint(t *testing.T) {
	if u, err := ToUint("9223372036854775808"); err != nil || u != 9223372036854775808 {
		t.Errorf("ToUint(MaxInt64+1) = %d, %v; want 9223372036854775808, nil", u, err)
	}
	if u, err := ToUint(uint64(18446744073709551615)); err != nil || u != 18446744073709551615 {
		t.Errorf("ToUint(MaxUint64) = %d, %v; want max, nil", u, err)
	}
	if u, err := ToUint(42); err != nil || u != 42 {
		t.Errorf("ToUint(42) = %d, %v; want 42, nil", u, err)
	}
	if u, err := ToUint("12.0"); err != nil || u != 12 {
		t.Errorf("ToUint(\"12.0\") = %d, %v; want 12, nil", u, err)
	}
	for _, bad := range []any{-1, int64(-5), "notnum", "-3", 1.5e30, "99999999999999999999999"} {
		if _, err := ToUint(bad); err == nil {
			t.Errorf("ToUint(%v) must error", bad)
		}
	}
	type Count uint32
	if u, err := ToUint(Count(7)); err != nil || u != 7 {
		t.Errorf("ToUint(named uint) = %d, %v; want 7, nil", u, err)
	}
}

// uintptr converts via ToInt/ToUint rather than hitting the error default.
func TestUintptrConversions(t *testing.T) {
	if i, err := ToInt(uintptr(5)); err != nil || i != 5 {
		t.Errorf("ToInt(uintptr(5)) = %d, %v; want 5, nil", i, err)
	}
	if u, err := ToUint(uintptr(5)); err != nil || u != 5 {
		t.Errorf("ToUint(uintptr(5)) = %d, %v; want 5, nil", u, err)
	}
}

func TestToFloat(t *testing.T) {
	if got, err := ToFloat("3.14"); err != nil || got != 3.14 {
		t.Errorf("ToFloat(3.14)=%v,%v", got, err)
	}
	if _, err := ToFloat("nope"); err == nil {
		t.Error("ToFloat(nope) should error")
	}
}

func TestToFloatRejectsNonFiniteStrings(t *testing.T) {
	for _, s := range []string{"NaN", "Inf", "+Inf", "-Inf", "Infinity"} {
		if _, err := ToFloat(s); err == nil {
			t.Errorf("ToFloat(%q) should reject non-finite input", s)
		}
	}
	if f, err := ToFloat("3.5"); err != nil || f != 3.5 {
		t.Errorf(`ToFloat("3.5") = %v, %v`, f, err)
	}
}

// ToFloat rejects native NaN/Inf too, per the "non-finite is rejected" contract.
func TestToFloatRejectsNativeNonFinite(t *testing.T) {
	for _, c := range []any{
		math.NaN(),
		math.Inf(1),
		math.Inf(-1),
		float32(math.Inf(1)),
	} {
		if got, err := ToFloat(c); err == nil {
			t.Errorf("ToFloat(%v) should reject a non-finite native float, got %v", c, got)
		}
	}
	if f, err := ToFloat(float32(1.5)); err != nil || f != 1.5 {
		t.Errorf("ToFloat(float32 1.5) = %v, %v", f, err)
	}
}

func TestToBool(t *testing.T) {
	cases := []struct {
		in   any
		want bool
		ok   bool
	}{
		{true, true, true},
		{"yes", true, true},
		{"off", false, true},
		{1, true, true},
		{0, false, true},
		{"maybe", false, false},
	}
	for _, c := range cases {
		got, err := ToBool(c.in)
		if (err == nil) != c.ok {
			t.Errorf("ToBool(%v) err=%v want ok=%v", c.in, err, c.ok)
			continue
		}
		if c.ok && got != c.want {
			t.Errorf("ToBool(%v)=%v want %v", c.in, got, c.want)
		}
	}
}

func TestToBoolFractional(t *testing.T) {
	if b, err := ToBool(0.5); err != nil || !b {
		t.Errorf("ToBool(0.5) should be true (non-zero), got %v, %v", b, err)
	}
	if b, _ := ToBool(0.0); b {
		t.Error("ToBool(0.0) should be false")
	}
}

// Converters handle named scalar types via the reflect underlying fallback.
func TestConvNamedScalarTypes(t *testing.T) {
	type MyInt int
	type MyUint uint
	type MyFloat float64
	type MyStr string
	type MyBool bool
	type MyBytes []byte

	if i, err := ToInt(MyInt(5)); err != nil || i != 5 {
		t.Errorf("ToInt(MyInt(5)) = %d, %v; want 5, nil", i, err)
	}
	if i, err := ToInt(MyUint(7)); err != nil || i != 7 {
		t.Errorf("ToInt(MyUint(7)) = %d, %v; want 7, nil", i, err)
	}
	if i, err := ToInt(MyStr("42")); err != nil || i != 42 {
		t.Errorf("ToInt(MyStr(42)) = %d, %v; want 42, nil", i, err)
	}
	if i, err := ToInt(MyBytes("99")); err != nil || i != 99 {
		t.Errorf("ToInt(MyBytes(99)) = %d, %v; want 99, nil", i, err)
	}
	if f, err := ToFloat(MyFloat(2.5)); err != nil || f != 2.5 {
		t.Errorf("ToFloat(MyFloat(2.5)) = %v, %v; want 2.5, nil", f, err)
	}
	if f, err := ToFloat(MyStr("3.14")); err != nil || f != 3.14 {
		t.Errorf("ToFloat(MyStr(3.14)) = %v, %v; want 3.14, nil", f, err)
	}
	if b, err := ToBool(MyBool(true)); err != nil || !b {
		t.Errorf("ToBool(MyBool(true)) = %v, %v; want true, nil", b, err)
	}
	if b, err := ToBool(MyStr("yes")); err != nil || !b {
		t.Errorf("ToBool(MyStr(yes)) = %v, %v; want true, nil", b, err)
	}
	if tm, err := ToTime(MyStr("2024-01-02")); err != nil || tm.IsZero() {
		t.Errorf("ToTime(MyStr date) = %v, %v; want parsed, nil", tm, err)
	}
	// underlying re-dispatch still applies the finite/range checks
	if _, err := ToFloat(MyStr("NaN")); err == nil {
		t.Error("ToFloat(MyStr(NaN)) must still reject non-finite")
	}
}

func TestToTime(t *testing.T) {
	got, err := ToTime("2023-01-02")
	if err != nil {
		t.Fatalf("ToTime err: %v", err)
	}
	if got.Year() != 2023 || got.Month() != time.January || got.Day() != 2 {
		t.Errorf("ToTime got %v", got)
	}
	if _, err := ToTime("not-a-date"); err == nil {
		t.Error("ToTime(not-a-date) should error")
	}
}

func TestToTimeIntUTC(t *testing.T) {
	ut, err := ToTime(int64(1700000000))
	if err != nil {
		t.Fatal(err)
	}
	if ut.Location() != time.UTC {
		t.Errorf("ToTime(int) should use UTC, got %v", ut.Location())
	}
}

// time.Time inputs normalize to UTC so int/string/time.Time paths agree.
func TestToTimeValueUTC(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*3600)
	in := time.Date(2024, 1, 2, 3, 4, 5, 0, loc)
	out, err := ToTime(in)
	if err != nil {
		t.Fatal(err)
	}
	if out.Location() != time.UTC {
		t.Errorf("ToTime(time.Time) location = %v, want UTC", out.Location())
	}
	if !out.Equal(in) {
		t.Errorf("ToTime(time.Time) changed the instant: %v vs %v", out, in)
	}

	// offset strings normalize to UTC too
	s, err := ToTime("2024-01-02T03:04:05+08:00")
	if err != nil {
		t.Fatal(err)
	}
	if s.Location() != time.UTC {
		t.Errorf("ToTime(string with offset) location = %v, want UTC", s.Location())
	}
}

// ToTime has no float/bool case, so such inputs reject cleanly in one re-dispatch
// rather than recursing through the underlying fallback into a stack overflow.
func TestToTimeNoInfiniteRecursion(t *testing.T) {
	type NamedFloat float64
	type NamedBool bool
	cases := []any{1.5, float32(1.5), true, false, NamedFloat(1.5), NamedBool(true)}
	for _, c := range cases {
		if _, err := ToTime(c); err == nil {
			t.Errorf("ToTime(%T %v) should return a clean error, not recurse", c, c)
		}
	}
	type NamedStr string
	type NamedTS int64
	if _, err := ToTime(NamedStr("2024-01-02")); err != nil {
		t.Errorf("ToTime(named string date) should parse: %v", err)
	}
	if tm, err := ToTime(NamedTS(1700000000)); err != nil || tm.IsZero() {
		t.Errorf("ToTime(named unix seconds) should parse: %v", err)
	}
}

// TestAllNumericKinds exercises every type-switch arm of each converter.
func TestAllNumericKinds(t *testing.T) {
	sevens := []any{
		int(7), int8(7), int16(7), int32(7), int64(7),
		uint(7), uint8(7), uint16(7), uint32(7), uint64(7),
		float32(7), float64(7),
	}
	for _, v := range sevens {
		if got, err := ToInt(v); err != nil || got != 7 {
			t.Errorf("ToInt(%T %v) = %d, %v; want 7", v, v, got, err)
		}
		if got, err := ToUint(v); err != nil || got != 7 {
			t.Errorf("ToUint(%T %v) = %d, %v; want 7", v, v, got, err)
		}
		if got, err := ToFloat(v); err != nil || got != 7 {
			t.Errorf("ToFloat(%T %v) = %v, %v; want 7", v, v, got, err)
		}
		if got, err := ToBool(v); err != nil || !got {
			t.Errorf("ToBool(%T %v) = %v, %v; want true", v, v, got, err)
		}
	}

	// uintptr arm exists in the integer converters only
	if got, err := ToInt(uintptr(7)); err != nil || got != 7 {
		t.Errorf("ToInt(uintptr) = %d, %v; want 7", got, err)
	}
	if got, err := ToUint(uintptr(7)); err != nil || got != 7 {
		t.Errorf("ToUint(uintptr) = %d, %v; want 7", got, err)
	}

	if got, _ := ToInt(true); got != 1 {
		t.Errorf("ToInt(true) = %d, want 1", got)
	}
	if got, _ := ToUint(true); got != 1 {
		t.Errorf("ToUint(true) = %d, want 1", got)
	}
	if got, _ := ToFloat(true); got != 1 {
		t.Errorf("ToFloat(true) = %v, want 1", got)
	}

	if got, _ := ToInt([]byte("7")); got != 7 {
		t.Errorf("ToInt([]byte) = %d, want 7", got)
	}
	if got, _ := ToUint([]byte("7")); got != 7 {
		t.Errorf("ToUint([]byte) = %d, want 7", got)
	}
	if got, _ := ToFloat([]byte("7")); got != 7 {
		t.Errorf("ToFloat([]byte) = %v, want 7", got)
	}
	if got, _ := ToBool([]byte("true")); !got {
		t.Error(`ToBool([]byte("true")) = false, want true`)
	}

	// zero numerics -> ToBool false
	for _, v := range []any{int(0), int8(0), uint(0), uint8(0), float32(0), float64(0)} {
		if got, _ := ToBool(v); got {
			t.Errorf("ToBool(%T 0) = true, want false", v)
		}
	}

	// nil -> zero value, no error
	if got, err := ToInt(nil); got != 0 || err != nil {
		t.Errorf("ToInt(nil) = %d, %v; want 0, nil", got, err)
	}
	if got, err := ToFloat(nil); got != 0 || err != nil {
		t.Errorf("ToFloat(nil) = %v, %v; want 0, nil", got, err)
	}
	if got, err := ToBool(nil); got || err != nil {
		t.Errorf("ToBool(nil) = %v, %v; want false, nil", got, err)
	}
}
