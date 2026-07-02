package validator

import (
	"context"
	"testing"
)

func TestNumericNonFinite(t *testing.T) {
	num := &numericRule{}
	for _, s := range []string{"Inf", "+Inf", "-Inf", "Infinity", "NaN", "nan"} {
		if num.Passes(fakeField{val: rvOf(s)}) {
			t.Errorf("numeric %q should be false", s)
		}
	}
	if !num.Passes(fakeField{val: rvOf("3.14")}) {
		t.Error("numeric 3.14 should pass")
	}
}

func TestSizeBlankThreshold(t *testing.T) {
	if (&minRule{}).Passes(fakeField{val: rvOf("ab"), attrs: []string{""}}) {
		t.Error("min with blank threshold should fail")
	}
	if (&betweenRule{}).Passes(fakeField{val: rvOf(5), attrs: []string{"", "10"}}) {
		t.Error("between with blank lower bound should fail")
	}
}

func TestBooleanRejectsNonBinary(t *testing.T) {
	b := &booleanRule{}
	for _, v := range []any{2, -5, 0.5, 2.0} {
		if b.Passes(fakeField{val: rvOf(v)}) {
			t.Errorf("boolean %v should fail", v)
		}
	}
	if !b.Passes(fakeField{val: rvOf(1)}) {
		t.Error("boolean 1 should pass")
	}
	if !b.Passes(fakeField{val: rvOf("true")}) {
		t.Error(`boolean "true" should pass`)
	}
	if b.Passes(fakeField{val: rvOf("maybe")}) {
		t.Error(`boolean "maybe" should fail`)
	}
	// blank is not a boolean, matching numeric/date
	for _, v := range []any{"   ", "\t\n", []byte("  ")} {
		if b.Passes(fakeField{val: rvOf(v)}) {
			t.Errorf("boolean %q should fail: blank is not a boolean", v)
		}
	}
}

// number matches numeric's trimming and accepts uint64-range strings like the
// native uint64 branch does.
func TestNumberStringConsistency(t *testing.T) {
	n := &numberRule{}
	if !n.Passes(fakeField{val: rvOf(" 42 ")}) {
		t.Error(`number " 42 " should pass: trimmed like numeric`)
	}
	if !n.Passes(fakeField{val: rvOf("18446744073709551615")}) {
		t.Error("number accepts uint64-range strings like native uint64 values")
	}
	if n.Passes(fakeField{val: rvOf("18446744073709551616")}) {
		t.Error("number rejects beyond-uint64 strings")
	}
}

// numeric is strict decimal: Go literal syntax is not form data.
func TestNumericStrictDecimal(t *testing.T) {
	num := &numericRule{}
	for _, s := range []string{"1_000", "0x1F", "0x1p4", "1e3", "1E-2", ".", "5.", ".5", "+"} {
		if num.Passes(fakeField{val: rvOf(s)}) {
			t.Errorf("numeric %q should fail under strict decimal parsing", s)
		}
	}
	for _, s := range []string{"42", "-7", "+3", "3.14", " 10 ", "-0.5"} {
		if !num.Passes(fakeField{val: rvOf(s)}) {
			t.Errorf("numeric %q should pass", s)
		}
	}
}

func TestNumberRejectsNonInteger(t *testing.T) {
	n := &numberRule{}
	for _, v := range []any{"  ", "1e3", "3.0", "0x1F", 3.5} {
		if n.Passes(fakeField{val: rvOf(v)}) {
			t.Errorf("number %v should fail", v)
		}
	}
	for _, v := range []any{float64(3.0), 42, "42", "-7"} {
		if !n.Passes(fakeField{val: rvOf(v)}) {
			t.Errorf("number %v should pass", v)
		}
	}
}

func TestMin(t *testing.T) {
	r := &minRule{}
	if r.Signature() != "min" {
		t.Fatalf("signature = %q", r.Signature())
	}
	cases := []struct {
		name  string
		val   any
		attrs []string
		want  bool
	}{
		{"number equal", 3, []string{"3"}, true},
		{"number above", 5, []string{"3"}, true},
		{"number below", 2, []string{"3"}, false},
		{"float below", 2.5, []string{"3"}, false},
		{"numeric string counts runes by default", "10", []string{"3"}, false},
		{"string length equal", "abc", []string{"3"}, true},
		{"string length below", "ab", []string{"3"}, false},
		{"slice length above", []int{1, 2, 3, 4}, []string{"3"}, true},
		{"empty value passes", "", []string{"3"}, true},
		{"nil passes", nil, []string{"3"}, true},
		{"missing arg", "abc", []string{}, false},
		{"bad arg", "abc", []string{"x"}, false},
		{"unmeasurable struct", struct{}{}, []string{"3"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := r.Passes(fakeField{name: "x", val: rvOf(c.val), attrs: c.attrs})
			if got != c.want {
				t.Errorf("Passes(%v, %v) = %v, want %v", c.val, c.attrs, got, c.want)
			}
		})
	}
}

func TestMax(t *testing.T) {
	r := &maxRule{}
	if r.Signature() != "max" {
		t.Fatalf("signature = %q", r.Signature())
	}
	cases := []struct {
		name  string
		val   any
		attrs []string
		want  bool
	}{
		{"number equal", 10, []string{"10"}, true},
		{"number below", 5, []string{"10"}, true},
		{"number above", 11, []string{"10"}, false},
		{"numeric string counts runes by default", "20", []string{"10"}, true},
		{"string length equal", "abcde", []string{"5"}, true},
		{"string length above", "abcdef", []string{"5"}, false},
		{"empty passes", "", []string{"5"}, true},
		{"missing arg", "abc", []string{}, false},
		{"bad arg", "abc", []string{"x"}, false},
		{"unmeasurable struct", struct{}{}, []string{"5"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := r.Passes(fakeField{name: "x", val: rvOf(c.val), attrs: c.attrs})
			if got != c.want {
				t.Errorf("Passes(%v, %v) = %v, want %v", c.val, c.attrs, got, c.want)
			}
		})
	}
}

func TestBetween(t *testing.T) {
	r := &betweenRule{}
	if r.Signature() != "between" {
		t.Fatalf("signature = %q", r.Signature())
	}
	cases := []struct {
		name  string
		val   any
		attrs []string
		want  bool
	}{
		{"in range", 5, []string{"1", "10"}, true},
		{"lower bound", 1, []string{"1", "10"}, true},
		{"upper bound", 10, []string{"1", "10"}, true},
		{"below", 0, []string{"1", "10"}, true}, // 0 is empty -> passes
		{"above", 11, []string{"1", "10"}, false},
		{"string length in range", "abc", []string{"2", "5"}, true},
		{"string length below", "a", []string{"2", "5"}, false},
		{"empty passes", "", []string{"1", "10"}, true},
		{"missing args", 5, []string{"1"}, false},
		{"bad first arg", 5, []string{"x", "10"}, false},
		{"bad second arg", 5, []string{"1", "y"}, false},
		{"unmeasurable struct", struct{}{}, []string{"1", "10"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := r.Passes(fakeField{name: "x", val: rvOf(c.val), attrs: c.attrs})
			if got != c.want {
				t.Errorf("Passes(%v, %v) = %v, want %v", c.val, c.attrs, got, c.want)
			}
		})
	}
}

func TestGt(t *testing.T) {
	r := &gtRule{}
	if r.Signature() != "gt" {
		t.Fatalf("signature = %q", r.Signature())
	}
	cases := []struct {
		name  string
		val   any
		attrs []string
		want  bool
	}{
		{"above", 5, []string{"3"}, true},
		{"equal fails", 3, []string{"3"}, false},
		{"below fails", 2, []string{"3"}, false},
		{"string length above", "abcd", []string{"3"}, true},
		{"string length equal fails", "abc", []string{"3"}, false},
		{"empty passes", "", []string{"3"}, true},
		{"missing arg", 5, []string{}, false},
		{"bad arg", 5, []string{"x"}, false},
		{"unmeasurable struct", struct{}{}, []string{"3"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := r.Passes(fakeField{name: "x", val: rvOf(c.val), attrs: c.attrs})
			if got != c.want {
				t.Errorf("Passes(%v, %v) = %v, want %v", c.val, c.attrs, got, c.want)
			}
		})
	}
}

func TestGte(t *testing.T) {
	r := &gteRule{}
	if r.Signature() != "gte" {
		t.Fatalf("signature = %q", r.Signature())
	}
	cases := []struct {
		name  string
		val   any
		attrs []string
		want  bool
	}{
		{"above", 5, []string{"3"}, true},
		{"equal", 3, []string{"3"}, true},
		{"below fails", 2, []string{"3"}, false},
		{"string length equal", "abc", []string{"3"}, true},
		{"empty passes", "", []string{"3"}, true},
		{"missing arg", 5, []string{}, false},
		{"bad arg", 5, []string{"x"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := r.Passes(fakeField{name: "x", val: rvOf(c.val), attrs: c.attrs})
			if got != c.want {
				t.Errorf("Passes(%v, %v) = %v, want %v", c.val, c.attrs, got, c.want)
			}
		})
	}
}

func TestLt(t *testing.T) {
	r := &ltRule{}
	if r.Signature() != "lt" {
		t.Fatalf("signature = %q", r.Signature())
	}
	cases := []struct {
		name  string
		val   any
		attrs []string
		want  bool
	}{
		{"below", 2, []string{"3"}, true},
		{"equal fails", 3, []string{"3"}, false},
		{"above fails", 5, []string{"3"}, false},
		{"string length below", "ab", []string{"3"}, true},
		{"empty passes", "", []string{"3"}, true},
		{"missing arg", 5, []string{}, false},
		{"bad arg", 5, []string{"x"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := r.Passes(fakeField{name: "x", val: rvOf(c.val), attrs: c.attrs})
			if got != c.want {
				t.Errorf("Passes(%v, %v) = %v, want %v", c.val, c.attrs, got, c.want)
			}
		})
	}
}

func TestLte(t *testing.T) {
	r := &lteRule{}
	if r.Signature() != "lte" {
		t.Fatalf("signature = %q", r.Signature())
	}
	cases := []struct {
		name  string
		val   any
		attrs []string
		want  bool
	}{
		{"below", 2, []string{"3"}, true},
		{"equal", 3, []string{"3"}, true},
		{"above fails", 5, []string{"3"}, false},
		{"string length equal", "abc", []string{"3"}, true},
		{"empty passes", "", []string{"3"}, true},
		{"missing arg", 5, []string{}, false},
		{"bad arg", 5, []string{"x"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := r.Passes(fakeField{name: "x", val: rvOf(c.val), attrs: c.attrs})
			if got != c.want {
				t.Errorf("Passes(%v, %v) = %v, want %v", c.val, c.attrs, got, c.want)
			}
		})
	}
}

func TestLen(t *testing.T) {
	r := &lenRule{}
	if r.Signature() != "len" {
		t.Fatalf("signature = %q", r.Signature())
	}
	cases := []struct {
		name  string
		val   any
		attrs []string
		want  bool
	}{
		{"number equal", 6, []string{"6"}, true},
		{"number not equal", 5, []string{"6"}, false},
		{"string length equal", "abcdef", []string{"6"}, true},
		{"string length not equal", "abc", []string{"6"}, false},
		{"slice length equal", []int{1, 2}, []string{"2"}, true},
		{"empty passes", "", []string{"6"}, true},
		{"missing arg", "abc", []string{}, false},
		{"bad arg", "abc", []string{"x"}, false},
		{"unmeasurable struct", struct{}{}, []string{"6"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := r.Passes(fakeField{name: "x", val: rvOf(c.val), attrs: c.attrs})
			if got != c.want {
				t.Errorf("Passes(%v, %v) = %v, want %v", c.val, c.attrs, got, c.want)
			}
		})
	}
}

func TestSize(t *testing.T) {
	r := &sizeRule{}
	if r.Signature() != "size" {
		t.Fatalf("signature = %q", r.Signature())
	}
	cases := []struct {
		name  string
		val   any
		attrs []string
		want  bool
	}{
		{"number equal", 6, []string{"6"}, true},
		{"number not equal", 5, []string{"6"}, false},
		{"string length equal", "abcdef", []string{"6"}, true},
		{"empty passes", "", []string{"6"}, true},
		{"missing arg", "abc", []string{}, false},
		{"bad arg", "abc", []string{"x"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := r.Passes(fakeField{name: "x", val: rvOf(c.val), attrs: c.attrs})
			if got != c.want {
				t.Errorf("Passes(%v, %v) = %v, want %v", c.val, c.attrs, got, c.want)
			}
		})
	}
}

func TestDigits(t *testing.T) {
	r := &digitsRule{}
	if r.Signature() != "digits" {
		t.Fatalf("signature = %q", r.Signature())
	}
	cases := []struct {
		name  string
		val   any
		attrs []string
		want  bool
	}{
		{"all digits exact", "1234", []string{"4"}, true},
		{"all digits wrong length", "123", []string{"4"}, false},
		{"contains letter", "12a4", []string{"4"}, false},
		{"contains dot", "12.4", []string{"4"}, false},
		{"negative sign fails", "-123", []string{"4"}, false},
		{"int value as string", 1234, []string{"4"}, true},
		{"empty passes", "", []string{"4"}, true},
		{"missing arg", "1234", []string{}, false},
		{"bad arg", "1234", []string{"x"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := r.Passes(fakeField{name: "x", val: rvOf(c.val), attrs: c.attrs})
			if got != c.want {
				t.Errorf("Passes(%v, %v) = %v, want %v", c.val, c.attrs, got, c.want)
			}
		})
	}
}

func TestNumeric(t *testing.T) {
	r := &numericRule{}
	if r.Signature() != "numeric" {
		t.Fatalf("signature = %q", r.Signature())
	}
	cases := []struct {
		name string
		val  any
		want bool
	}{
		{"int", 5, true},
		{"float", 3.14, true},
		{"numeric string", "12.5", true},
		{"negative numeric string", "-7", true},
		{"non-numeric string", "abc", false},
		{"slice not numeric", []int{1, 2}, false},
		{"empty passes", "", true},
		{"nil passes", nil, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := r.Passes(fakeField{name: "x", val: rvOf(c.val)})
			if got != c.want {
				t.Errorf("Passes(%v) = %v, want %v", c.val, got, c.want)
			}
		})
	}
}

func TestNumber(t *testing.T) {
	r := &numberRule{}
	if r.Signature() != "number" {
		t.Fatalf("signature = %q", r.Signature())
	}
	cases := []struct {
		name string
		val  any
		want bool
	}{
		{"int", 5, true},
		{"int string", "42", true},
		{"float string fails", "3.14", false},
		{"non-numeric string fails", "abc", false},
		{"empty passes", "", true},
		{"nil passes", nil, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := r.Passes(fakeField{name: "x", val: rvOf(c.val)})
			if got != c.want {
				t.Errorf("Passes(%v) = %v, want %v", c.val, got, c.want)
			}
		})
	}
}

func TestBoolean(t *testing.T) {
	r := &booleanRule{}
	if r.Signature() != "boolean" {
		t.Fatalf("signature = %q", r.Signature())
	}
	cases := []struct {
		name string
		val  any
		want bool
	}{
		{"bool true", true, true},
		{"int one", 1, true},
		{"string true", "true", true},
		{"string yes", "yes", true},
		{"string on", "on", true},
		{"string off", "off", true},
		{"invalid string fails", "maybe", false},
		{"non-convertible struct fails", struct{}{}, false},
		{"empty passes", "", true},
		{"nil passes", nil, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := r.Passes(fakeField{name: "x", val: rvOf(c.val)})
			if got != c.want {
				t.Errorf("Passes(%v) = %v, want %v", c.val, got, c.want)
			}
		})
	}
}

func TestNumericMessages(t *testing.T) {
	msgs := map[string]string{
		"min":     (&minRule{}).Message(),
		"max":     (&maxRule{}).Message(),
		"between": (&betweenRule{}).Message(),
		"gt":      (&gtRule{}).Message(),
		"gte":     (&gteRule{}).Message(),
		"lt":      (&ltRule{}).Message(),
		"lte":     (&lteRule{}).Message(),
		"len":     (&lenRule{}).Message(),
		"size":    (&sizeRule{}).Message(),
		"digits":  (&digitsRule{}).Message(),
		"numeric": (&numericRule{}).Message(),
		"number":  (&numberRule{}).Message(),
		"boolean": (&booleanRule{}).Message(),
	}
	for sig, msg := range msgs {
		if msg == "" {
			t.Errorf("rule %q has empty message", sig)
		}
	}
}

// Size rules on strings count runes by default; a numeric/number assertion on
// the expression's AND spine flips them to numeric comparison (the compiler
// swaps in the hinted variant).
func TestSizeStringSemantics(t *testing.T) {
	ctx := context.Background()

	// the README password shape: min:8 must reject a short numeric password
	pw := Map(map[string]any{"pwd": "999"}, map[string]string{"pwd": "required && min:8"})
	pw.Validate(ctx)
	if !pw.Errors().Has("pwd") {
		t.Error(`min:8 on "999" must fail by rune count — numeric content must not bypass a length constraint`)
	}

	// form-style numeric field: numeric && gte:18 compares by value
	age := Map(map[string]any{"age": "30"}, map[string]string{"age": "numeric && gte:18"})
	age.Validate(ctx)
	if age.Fails() {
		t.Errorf(`numeric && gte:18 on "30" must compare numerically, got %v`, age.Errors().All())
	}
	young := Map(map[string]any{"age": "9"}, map[string]string{"age": "numeric && gte:18"})
	young.Validate(ctx)
	if !young.Errors().Has("age") {
		t.Error(`numeric && gte:18 on "9" must fail numerically (9 < 18), not pass by length`)
	}

	// the hint applies only on the AND spine, not inside || or !
	or := Map(map[string]any{"v": "999"}, map[string]string{"v": "(numeric || alpha) && min:8"})
	or.Validate(ctx)
	if !or.Errors().Has("v") {
		t.Error("a numeric leaf inside || must not flip min to numeric comparison")
	}

	// multibyte strings count runes, not bytes
	uni := Map(map[string]any{"name": "héllo"}, map[string]string{"name": "len:5"})
	uni.Validate(ctx)
	if uni.Fails() {
		t.Errorf("len counts runes: héllo is 5, got %v", uni.Errors().All())
	}
}

// Integer comparisons stay exact above 2^53 (no float64 rounding).
func TestSizeIntegerPrecision(t *testing.T) {
	ctx := context.Background()
	const big = int64(9007199254740993) // 2^53 + 1: rounds to 2^53 in float64

	eq := Map(map[string]any{"n": big}, map[string]string{"n": "gte:9007199254740993"})
	eq.Validate(ctx)
	if eq.Fails() {
		t.Errorf("gte at 2^53+1 must compare exactly, got %v", eq.Errors().All())
	}
	lt := Map(map[string]any{"n": big - 1}, map[string]string{"n": "lt:9007199254740993"})
	lt.Validate(ctx)
	if lt.Fails() {
		t.Errorf("lt at 2^53 boundary must compare exactly, got %v", lt.Errors().All())
	}
	gt := Map(map[string]any{"n": big}, map[string]string{"n": "gt:9007199254740992"})
	gt.Validate(ctx)
	if gt.Fails() {
		t.Errorf("gt at 2^53 boundary must compare exactly, got %v", gt.Errors().All())
	}

	// uint64 above MaxInt64 with an exact threshold
	u := Map(map[string]any{"n": uint64(18446744073709551615)}, map[string]string{"n": "gte:18446744073709551615"})
	u.Validate(ctx)
	if u.Fails() {
		t.Errorf("MaxUint64 gte:MaxUint64 must pass exactly, got %v", u.Errors().All())
	}

	// numeric strings under the hint also compare exactly
	s := Map(map[string]any{"n": "9007199254740993"}, map[string]string{"n": "numeric && gt:9007199254740992"})
	s.Validate(ctx)
	if s.Fails() {
		t.Errorf("hinted numeric string must compare exactly above 2^53, got %v", s.Errors().All())
	}
}
