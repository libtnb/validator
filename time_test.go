package validator

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestAfterBeforeLiteral(t *testing.T) {
	cases := []struct {
		rule Rule
		val  any
		arg  string
		want bool
	}{
		{&afterRule{}, "2026-01-02", "2026-01-01", true},
		{&afterRule{}, "2026-01-01", "2026-01-01", false},
		{&afterRule{}, "2025-12-31", "2026-01-01", false},
		{&afterOrEqualRule{}, "2026-01-01", "2026-01-01", true},
		{&beforeRule{}, "2025-12-31", "2026-01-01", true},
		{&beforeRule{}, "2026-01-01", "2026-01-01", false},
		{&beforeOrEqualRule{}, "2026-01-01", "2026-01-01", true},
		// time.Time value against a literal
		{&afterRule{}, time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), "2026-01-01", true},
		// empty passes (omitempty)
		{&afterRule{}, "", "2026-01-01", true},
		{&afterRule{}, nil, "2026-01-01", true},
		// unparseable value or arg fails closed
		{&afterRule{}, "garbage", "2026-01-01", false},
		{&afterRule{}, "2026-01-02", "garbage", false},
	}
	for _, c := range cases {
		got := c.rule.Passes(fakeField{val: reflect.ValueOf(c.val), attrs: []string{c.arg}})
		if got != c.want {
			t.Errorf("%s(%v vs %q)=%v want %v", c.rule.Signature(), c.val, c.arg, got, c.want)
		}
	}
}

func TestAfterSiblingReference(t *testing.T) {
	start := fakeField{val: reflect.ValueOf("2026-05-01")}
	f := fakeField{
		val:      reflect.ValueOf("2026-06-01"),
		attrs:    []string{"Start"},
		siblings: map[string]Field{"Start": start},
	}
	if !(&afterRule{}).Passes(f) {
		t.Error("after:Start should compare against the sibling and pass")
	}
	// sibling exists but is not a time -> fail closed
	bad := fakeField{
		val:      reflect.ValueOf("2026-06-01"),
		attrs:    []string{"Start"},
		siblings: map[string]Field{"Start": fakeField{val: reflect.ValueOf("nope")}},
	}
	if (&afterRule{}).Passes(bad) {
		t.Error("an unparseable sibling must fail closed")
	}
}

// A parseable literal date is ALWAYS the literal bound: map/JSON keys are
// user-controlled, so a sibling-first lookup would let an attacker submit a key
// named "2026-01-01" to shadow a hard-coded cutoff.
func TestDateLiteralCannotBeShadowed(t *testing.T) {
	rules := map[string]string{"expiry": "before:2026-01-01"}
	attack := Map(map[string]any{
		"expiry":     "2026-06-01",
		"2026-01-01": "2027-01-01", // decoy key matching the literal
	}, rules)
	attack.Validate(context.Background())
	if !attack.Fails() {
		t.Error("a user key matching the literal date must not shadow the cutoff")
	}
	good := Map(map[string]any{"expiry": "2025-06-01", "2026-01-01": "1990-01-01"}, rules)
	good.Validate(context.Background())
	if good.Fails() {
		t.Errorf("an in-range value must pass regardless of decoy keys: %v", good.Errors().All())
	}
}

// Struct-source nil collections are absent under sometimes BY DESIGN: after Go
// decoding, {"name":"x"} (missing key) and {"tags":null} both yield Tags==nil —
// indistinguishable — and failing them would break every PATCH body that omits
// the field. The precise null-vs-missing distinction lives in the JSON/Map
// entrypoints (key presence), covered by TestSometimesExplicitNull.
func TestSometimesStructNilCollectionSkips(t *testing.T) {
	type Patch struct {
		Tags []string `validate:"sometimes && required && min:1"`
	}
	v := NewValidator()
	vd := v.Struct(&Patch{})
	vd.Validate(context.Background())
	if vd.Fails() {
		t.Errorf("a nil struct collection is absent under sometimes: %v", vd.Errors().All())
	}
	// a non-nil (even empty) collection is present: the chain runs
	type Strict struct {
		Tags []string `validate:"sometimes && filled"`
	}
	present := v.Struct(&Strict{Tags: []string{}})
	present.Validate(context.Background())
	if !present.Fails() {
		t.Error("a non-nil empty collection is present: filled must run and reject it")
	}
}

func TestAfterEndToEndStruct(t *testing.T) {
	type Booking struct {
		Start string `validate:"required && date"`
		End   string `validate:"required && date && after:Start"`
	}
	v := NewValidator()
	ok := v.Struct(&Booking{Start: "2026-01-01", End: "2026-02-01"})
	ok.Validate(context.Background())
	if ok.Fails() {
		t.Fatalf("valid booking failed: %v", ok.Errors().All())
	}
	bad := v.Struct(&Booking{Start: "2026-02-01", End: "2026-01-01"})
	bad.Validate(context.Background())
	if !bad.Fails() {
		t.Fatal("End before Start must fail after:Start")
	}
	if !v.Valid(&Booking{Start: "2026-01-01", End: "2026-02-01"}) {
		t.Error("Valid() fast path disagrees with Validate")
	}
}

// An empty reference (blank string, zero time.Time, blank literal) cannot
// anchor a chronological assertion: fail closed instead of comparing to year 1
// (conv.ToTime("") returns the zero time with no error).
func TestTimeEmptyReferenceFailsClosed(t *testing.T) {
	blankSib := fakeField{
		val:      reflect.ValueOf("2026-01-01"),
		attrs:    []string{"Start"},
		siblings: map[string]Field{"Start": fakeField{val: reflect.ValueOf("")}},
	}
	if (&afterRule{}).Passes(blankSib) {
		t.Error("after:Start with a blank sibling must fail closed")
	}
	zeroSib := fakeField{
		val:      reflect.ValueOf("2026-01-01"),
		attrs:    []string{"Start"},
		siblings: map[string]Field{"Start": fakeField{val: reflect.ValueOf(time.Time{})}},
	}
	if (&afterRule{}).Passes(zeroSib) {
		t.Error("after:Start with a zero time.Time sibling must fail closed")
	}
	empty := Var("2026-01-01", "after:")
	empty.Validate(context.Background())
	if !empty.Fails() {
		t.Error("a blank literal arg must fail closed")
	}
	// gtfield: a zero-time sibling is an empty reference too (time.Time pair path)
	zeroPair := fakeField{
		val:      reflect.ValueOf(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		attrs:    []string{"Start"},
		siblings: map[string]Field{"Start": fakeField{val: reflect.ValueOf(time.Time{})}},
	}
	if (&gtFieldRule{}).Passes(zeroPair) {
		t.Error("gtfield with a zero time.Time sibling must fail closed")
	}
}

// gtfield family now compares time.Time pairs chronologically.
func TestGtFieldTime(t *testing.T) {
	earlier := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	later := earlier.Add(24 * time.Hour)
	sib := map[string]Field{"Start": fakeField{val: reflect.ValueOf(earlier)}}

	if !(&gtFieldRule{}).Passes(fakeField{val: reflect.ValueOf(later), attrs: []string{"Start"}, siblings: sib}) {
		t.Error("gtfield: later time must be greater than earlier")
	}
	if (&gtFieldRule{}).Passes(fakeField{val: reflect.ValueOf(earlier), attrs: []string{"Start"}, siblings: sib}) {
		t.Error("gtfield: equal times are not greater")
	}
	if !(&gteFieldRule{}).Passes(fakeField{val: reflect.ValueOf(earlier), attrs: []string{"Start"}, siblings: sib}) {
		t.Error("gtefield: equal times must pass")
	}
	if !(&ltFieldRule{}).Passes(fakeField{val: reflect.ValueOf(earlier), attrs: []string{"End"}, siblings: map[string]Field{"End": fakeField{val: reflect.ValueOf(later)}}}) {
		t.Error("ltfield: earlier time must be less than later")
	}
}
