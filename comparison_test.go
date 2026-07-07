package validator

import (
	"context"
	"reflect"
	"testing"
	"time"
)

// rvOf wraps v as a reflect.Value; nil yields an invalid Value (absent field).
func rvOf(v any) reflect.Value {
	if v == nil {
		return reflect.Value{}
	}
	return reflect.ValueOf(v)
}

func TestIn(t *testing.T) {
	r := &inRule{}
	if r.Signature() != "in" {
		t.Errorf("Signature()=%q", r.Signature())
	}
	cases := []struct {
		name  string
		val   any
		attrs []string
		want  bool
	}{
		{"present passes", "b", []string{"a", "b", "c"}, true},
		{"absent fails", "z", []string{"a", "b"}, false},
		{"numeric matches string form", 1, []string{"1", "2"}, true},
		{"empty value passes (omitempty)", "", []string{"a", "b"}, true},
		{"nil value passes (omitempty)", nil, []string{"a", "b"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := r.Passes(fakeField{val: rvOf(c.val), attrs: c.attrs}); got != c.want {
				t.Errorf("Passes(val=%v, attrs=%v)=%v, want %v", c.val, c.attrs, got, c.want)
			}
		})
	}
}

func TestNotIn(t *testing.T) {
	r := &notInRule{}
	if r.Signature() != "not_in" {
		t.Errorf("Signature()=%q", r.Signature())
	}
	cases := []struct {
		name  string
		val   any
		attrs []string
		want  bool
	}{
		{"absent passes", "z", []string{"a", "b", "c"}, true},
		{"present fails", "b", []string{"a", "b", "c"}, false},
		{"numeric matches string form fails", 1, []string{"1", "2"}, false},
		{"numeric absent passes", 3, []string{"1", "2"}, true},
		{"empty value passes (omitempty)", "", []string{"a", "b"}, true},
		{"nil value passes (omitempty)", nil, []string{"a", "b"}, true},
		{"no args passes", "anything", nil, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := r.Passes(fakeField{val: rvOf(c.val), attrs: c.attrs}); got != c.want {
				t.Errorf("Passes(val=%v, attrs=%v)=%v, want %v", c.val, c.attrs, got, c.want)
			}
		})
	}
}

func TestEq(t *testing.T) {
	r := &eqRule{}
	if r.Signature() != "eq" {
		t.Errorf("Signature()=%q", r.Signature())
	}
	cases := []struct {
		name  string
		val   any
		attrs []string
		want  bool
	}{
		{"equal passes", "foo", []string{"foo"}, true},
		{"not equal fails", "bar", []string{"foo"}, false},
		{"numeric equal via string passes", 1, []string{"1"}, true},
		{"numeric not equal fails", 2, []string{"1"}, false},
		{"no args fails", "foo", nil, false},
		{"empty value passes (omitempty)", "", []string{"foo"}, true},
		{"nil value passes (omitempty)", nil, []string{"foo"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := r.Passes(fakeField{val: rvOf(c.val), attrs: c.attrs}); got != c.want {
				t.Errorf("Passes(val=%v, attrs=%v)=%v, want %v", c.val, c.attrs, got, c.want)
			}
		})
	}
}

func TestNe(t *testing.T) {
	r := &neRule{}
	if r.Signature() != "ne" {
		t.Errorf("Signature()=%q", r.Signature())
	}
	cases := []struct {
		name  string
		val   any
		attrs []string
		want  bool
	}{
		{"not equal passes", "bar", []string{"foo"}, true},
		{"equal fails", "foo", []string{"foo"}, false},
		{"numeric equal via string fails", 1, []string{"1"}, false},
		{"numeric not equal passes", 2, []string{"1"}, true},
		{"no args passes", "foo", nil, true},
		{"empty value passes (omitempty)", "", []string{"foo"}, true},
		{"nil value passes (omitempty)", nil, []string{"foo"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := r.Passes(fakeField{val: rvOf(c.val), attrs: c.attrs}); got != c.want {
				t.Errorf("Passes(val=%v, attrs=%v)=%v, want %v", c.val, c.attrs, got, c.want)
			}
		})
	}
}

// Non-canonical numeric arg spellings match numeric values: ne:5.0 must
// reject float 5, not_in:5.0 must block it; string values stay literal.
func TestComparisonNumericArgNormalization(t *testing.T) {
	cases := []struct {
		rule  string
		val   any
		fails bool
	}{
		{"eq:5.0", 5.0, false},
		{"ne:5.0", 5.0, true},
		{"not_in:5.0", 5.0, true},
		{"in:5.0", 5.0, false},
		{"eq:5.0", 5, false},     // int 5 matches 5.0 numerically
		{"eq:5.0", "5.0", false}, // string compares literally
		{"eq:5.0", "5", true},    // string "5" is not the literal "5.0"
	}
	for _, c := range cases {
		vd := Var(c.val, c.rule)
		vd.Validate(context.Background())
		if vd.Fails() != c.fails {
			t.Errorf("Var(%v, %q): fails=%v want %v", c.val, c.rule, vd.Fails(), c.fails)
		}
	}
}

// eq/ne/in/not_in compare integers exactly: distinct values above 2^53 must
// never collide through float64 rounding of the arg.
func TestComparisonIntegerPrecision(t *testing.T) {
	ctx := context.Background()
	const boundary = int64(9007199254740992) // 2^53

	eq := Var(boundary, "eq:9007199254740993")
	eq.Validate(ctx)
	if !eq.Fails() {
		t.Error("eq must not match a distinct integer one ulp away above 2^53")
	}
	ne := Var(boundary, "ne:9007199254740993")
	ne.Validate(ctx)
	if ne.Fails() {
		t.Error("ne of a distinct >2^53 integer must pass")
	}
	ni := Var(boundary, "not_in:9007199254740993")
	ni.Validate(ctx)
	if ni.Fails() {
		t.Error("not_in must not block a distinct >2^53 integer")
	}
	hit := Var(boundary+1, "eq:9007199254740993")
	hit.Validate(ctx)
	if hit.Fails() {
		t.Error("eq of the exact >2^53 integer must match")
	}
	u := Var(uint64(18446744073709551615), "eq:18446744073709551614")
	u.Validate(ctx)
	if !u.Fails() {
		t.Error("uint64 eq must compare exactly at MaxUint64")
	}
}

// Comparison rules render named Stringer types via String(), byte-identical
// to conv.ToString: eq:5s must accept time.Duration(5s) and not_in must block it.
func TestComparisonNamedStringerType(t *testing.T) {
	d := 5 * time.Second
	eq := Var(d, "eq:5s")
	eq.Validate(context.Background())
	if eq.Fails() {
		t.Errorf("eq:5s must match a 5s Duration via its String() form, got %v", eq.Errors().All())
	}
	ni := Var(d, "not_in:5s,10s")
	ni.Validate(context.Background())
	if !ni.Fails() {
		t.Error("not_in:5s must block a 5s Duration — the Stringer form is the canonical rendering")
	}
	ct := Var(d, "contains:5s")
	ct.Validate(context.Background())
	if ct.Fails() {
		t.Error("contains sees the same String() rendering")
	}
}

func TestCaseInsensitiveComparisons(t *testing.T) {
	cases := []struct {
		r     Rule
		val   any
		attrs []string
		want  bool
	}{
		{&inCiRule{}, "ADMIN", []string{"admin", "user"}, true},
		{&inCiRule{}, "Admin", []string{"admin"}, true},
		{&inCiRule{}, "root", []string{"admin", "user"}, false},
		{&inCiRule{}, "", []string{"admin"}, true}, // omitempty
		{&eqIgnoreCaseRule{}, "Yes", []string{"yes"}, true},
		{&eqIgnoreCaseRule{}, "no", []string{"yes"}, false},
		{&eqIgnoreCaseRule{}, "", []string{"yes"}, true}, // omitempty
		{&neIgnoreCaseRule{}, "Yes", []string{"yes"}, false},
		{&neIgnoreCaseRule{}, "no", []string{"yes"}, true},
	}
	for _, c := range cases {
		if got := c.r.Passes(fakeField{val: reflect.ValueOf(c.val), attrs: c.attrs}); got != c.want {
			t.Errorf("%s(%v vs %v)=%v want %v", c.r.Signature(), c.val, c.attrs, got, c.want)
		}
	}
}
