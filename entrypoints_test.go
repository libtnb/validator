package validator

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/url"
	"testing"
	"time"
)

// embAddr's tagged fields are validated via flat promotion.
type embAddr struct {
	City string `validate:"required"`
}

func TestEmbeddedStructValidated(t *testing.T) {
	type User struct {
		Name string `validate:"required"`
		embAddr
	}
	// strict so the empty promoted City fails, proving it is in the plan
	v := MustNew(WithStrictRequired())

	bad := v.MustStruct(User{Name: "x"})
	bad.Validate(context.Background())
	if !bad.Fails() || !bad.Errors().Has("City") {
		t.Errorf("promoted embedded City should be required, got %v", bad.Errors().All())
	}

	ok := v.MustStruct(User{Name: "x", embAddr: embAddr{City: "NYC"}})
	ok.Validate(context.Background())
	if ok.Fails() {
		t.Errorf("valid embedded should pass: %v", ok.Errors().All())
	}
}

func TestValueScalarValidatable(t *testing.T) {
	vd := MustValue(42, "")
	if err := vd.AddRules("value", "min:100"); err != nil {
		t.Fatal(err)
	}
	vd.Validate(context.Background())
	if !vd.Fails() {
		t.Error("MustValue(42) with min:100 should fail")
	}
}

func TestJSONDecodeErrorSurfaced(t *testing.T) {
	if _, err := JSON(`[1,2,3]`, map[string]string{"name": "alpha"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("non-object error = %v", err)
	}

	if _, err := JSON(`{not json`, map[string]string{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid JSON error = %v", err)
	}
}

// boolean accepts a named bool type.
func TestBooleanNamedBool(t *testing.T) {
	type MyBool bool
	for _, val := range []any{MyBool(true), MyBool(false)} {
		v := MustValue(val, "boolean")
		v.Validate(context.Background())
		if v.Fails() {
			t.Errorf("a named bool (%v) should pass boolean, got %v", val, v.Errors().All())
		}
	}
}

type namedByteSlice []byte

// notblank on a blank named []byte must fail: it renders as text, not the
// byte-number form that would bypass the blank check.
func TestNotBlankNamedBytesBypass(t *testing.T) {
	blank := MustValue(namedByteSlice("   "), "notblank")
	blank.Validate(context.Background())
	if !blank.Fails() {
		t.Error("notblank on a blank named-[]byte must fail")
	}
	ok := MustValue(namedByteSlice("abc"), "notblank")
	ok.Validate(context.Background())
	if ok.Fails() {
		t.Errorf("notblank on a non-blank named-[]byte should pass, got %v", ok.Errors().All())
	}
}

// Stringify-colliding map keys (int 1 and string "1") must pick a deterministic survivor.
func TestAsStringMapDeterministic(t *testing.T) {
	var first bool
	for i := range 50 {
		m := map[any]any{1: "intval", "1": "strval"}
		vd := MustMap(m, nil)
		if err := vd.AddRules("1", "in:intval"); err != nil {
			t.Fatal(err)
		}
		vd.Validate(context.Background())
		if i == 0 {
			first = vd.Fails()
			continue
		}
		if vd.Fails() != first {
			t.Fatalf("nondeterministic verdict for stringify-colliding keys: run %d differs from first (%v)", i, first)
		}
	}
}

// Two distinct NaN float keys must pick a deterministic survivor via the value tiebreak.
func TestAsStringMapNaNDeterministic(t *testing.T) {
	first := -1
	for i := range 50 {
		m := map[float64]any{}
		m[math.NaN()] = "aaa"
		m[math.NaN()] = "bbb"
		vd := MustMap(m, nil)
		if err := vd.AddRules("NaN", "in:aaa"); err != nil {
			t.Fatal(err)
		}
		vd.Validate(context.Background())
		n := len(vd.Errors().All())
		if first == -1 {
			first = n
			continue
		}
		if n != first {
			t.Fatalf("nondeterministic NaN-key survivor: run %d has %d errors, first had %d", i, n, first)
		}
	}
}

// JSON uses UseNumber, so large integers keep full precision (float64 would round them).
func TestJSONLargeIntPrecision(t *testing.T) {
	type T struct {
		U uint64
		I int64
	}
	var out T
	vd := MustJSON(`{"U": 18446744073709551615, "I": 9007199254740993}`, map[string]string{})
	vd.Validate(context.Background())
	if err := vd.Bind(&out); err != nil {
		t.Fatal(err)
	}
	if out.U != 18446744073709551615 {
		t.Errorf("large uint64 from JSON lost precision: got %d", out.U)
	}
	if out.I != 9007199254740993 {
		t.Errorf("large int64 from JSON lost precision: got %d", out.I)
	}
	// trailing garbage after the object is rejected (json.Unmarshal parity)
	if _, err := JSON(`{"x":1}garbage`, map[string]string{"x": "required"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("trailing data error = %v", err)
	}
}

// JSON numbers behave uniformly with Map across string rules, binding, and not_in.
func TestJSONNumberUniformWithMap(t *testing.T) {
	// 5.0 must behave like 5, not the text "5.0"
	for _, rule := range []string{"number", "eq:5", "in:5", "digits:1", "ne:5", "not_in:5"} {
		j := MustJSON(`{"x":5.0}`, map[string]string{"x": rule})
		j.Validate(context.Background())
		m := MustMap(map[string]any{"x": 5.0}, map[string]string{"x": rule})
		m.Validate(context.Background())
		if j.Fails() != m.Fails() {
			t.Errorf("rule %q: JSON 5.0 fails=%v but Map 5.0 fails=%v (must be uniform)", rule, j.Fails(), m.Fails())
		}
	}

	// not_in:5 must block JSON 5.0
	nb := MustJSON(`{"role":5.0}`, map[string]string{"role": "not_in:5"})
	nb.Validate(context.Background())
	if !nb.Fails() {
		t.Error("not_in:5 must block JSON 5.0")
	}

	// a uint64 above MaxInt64 passes number from JSON, like native uint64
	big := MustJSON(`{"n":18446744073709551615}`, map[string]string{"n": "number"})
	big.Validate(context.Background())
	if big.Fails() {
		t.Errorf("a JSON uint64 above MaxInt64 should pass number, got %v", big.Errors().All())
	}

	// a JSON number binds to bool like a native numeric source
	type T struct{ B bool }
	var jb T
	if err := MustJSON(`{"B":2}`, nil).Bind(&jb); err != nil {
		t.Fatal(err)
	}
	var mb T
	_ = MustMap(map[string]any{"B": 2.0}, nil).Bind(&mb)
	if !jb.B || jb.B != mb.B {
		t.Errorf("JSON number->bool must match Map: jb.B=%v mb.B=%v (want both true)", jb.B, mb.B)
	}

	// large integers bind precisely, and a genuine float stays a float
	type N struct {
		U uint64
		F float64
	}
	var n N
	if err := MustJSON(`{"U":18446744073709551615,"F":5.5}`, nil).Bind(&n); err != nil {
		t.Fatal(err)
	}
	if n.U != 18446744073709551615 || n.F != 5.5 {
		t.Errorf("precise uint64 + float bind: got U=%d F=%v", n.U, n.F)
	}
}

// Trailing data after the top-level JSON value, including a stray close delimiter, must be rejected.
func TestJSONTrailingData(t *testing.T) {
	for _, bad := range []string{`{"x":1}]`, `{"x":1}}`, `{"x":1}{}`, `{"x":1} garbage`, `{"x":1},`} {
		if _, err := JSON(bad, map[string]string{"x": "required"}); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("malformed JSON %q error = %v", bad, err)
		}
	}
	ok := MustJSON("{\"x\":1}  \n\t", map[string]string{"x": "required"})
	ok.Validate(context.Background())
	if ok.Fails() {
		t.Errorf("valid JSON with trailing whitespace must be accepted, got %v", ok.Errors().All())
	}
}

// Map accepts any map type as a per-field source.
func TestMapConcreteValueType(t *testing.T) {
	vd := MustMap(map[string]string{"email": "not-an-email"}, nil)
	if err := vd.AddRules("email", "email"); err != nil {
		t.Fatal(err)
	}
	vd.Validate(context.Background())
	if !vd.Errors().Has("email") {
		t.Errorf("Map over a map[string]string should validate per key, got %v", vd.Errors().All())
	}
}

// a whitespace-only string is not numeric 0: numeric rejects it, size rules measure by length.
func TestWhitespaceNotNumeric(t *testing.T) {
	num := MustValue("   ", "numeric")
	num.Validate(context.Background())
	if !num.Fails() {
		t.Error("numeric must reject a whitespace-only string")
	}
	mn := MustValue("   ", "min:3") // 3 runes, sized by length not as numeric 0
	mn.Validate(context.Background())
	if mn.Fails() {
		t.Errorf("min:3 on a 3-space string should pass by length, got %v", mn.Errors().All())
	}
	mx := MustValue("   ", "max:1")
	mx.Validate(context.Background())
	if !mx.Fails() {
		t.Error("max:1 on a 3-space string should fail by length")
	}
}

// named string types (json.Number) are treated as numeric.
func TestNamedStringNumeric(t *testing.T) {
	vd := MustValue(json.Number("42"), "numeric && min:10")
	vd.Validate(context.Background())
	if vd.Fails() {
		t.Errorf("json.Number(42) should be numeric and >=10, got %v", vd.Errors().All())
	}
}

// date/datetime reject whitespace-only strings.
func TestDateWhitespace(t *testing.T) {
	d := MustValue("   ", "date")
	d.Validate(context.Background())
	if !d.Fails() {
		t.Error("date must reject a whitespace-only string")
	}
	dt := MustValue("   ", "datetime:2006-01-02")
	dt.Validate(context.Background())
	if !dt.Fails() {
		t.Error("datetime must reject a whitespace-only string")
	}
}

// boolean accepts named string types holding textual booleans.
func TestBooleanNamedString(t *testing.T) {
	type Status string
	for _, v := range []Status{"true", "false", "yes", "on", "0", "1"} {
		vd := MustValue(v, "boolean")
		vd.Validate(context.Background())
		if vd.Fails() {
			t.Errorf("boolean(Status(%q)) should pass", v)
		}
	}
	bad := MustValue(Status("maybe"), "boolean")
	bad.Validate(context.Background())
	if !bad.Fails() {
		t.Error("boolean(Status(maybe)) should fail")
	}
}

// number accepts named string types holding integers (json.Number).
func TestNumberNamedString(t *testing.T) {
	ok := MustValue(json.Number("5"), "number")
	ok.Validate(context.Background())
	if ok.Fails() {
		t.Errorf("number(json.Number(5)) should pass, got %v", ok.Errors().All())
	}
	bad := MustValue(json.Number("3.0"), "number")
	bad.Validate(context.Background())
	if !bad.Fails() {
		t.Error("number(json.Number(3.0)) should fail (not an integer)")
	}
}

// date/datetime accept a genuine time.Time value.
func TestDateAcceptsTimeValue(t *testing.T) {
	now := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	for _, rule := range []string{"date", "datetime", "datetime:2006-01-02"} {
		vd := MustValue(now, rule)
		vd.Validate(context.Background())
		if vd.Fails() {
			t.Errorf("%s should accept a time.Time value, got %v", rule, vd.Errors().All())
		}
	}
	// the zero time is empty, so omitempty skips it
	var zero time.Time
	z := MustValue(zero, "date")
	z.Validate(context.Background())
	if z.Fails() {
		t.Error("a zero time.Time is empty, so date should skip it (omitempty)")
	}
}

// []byte numbers are handled by value (numeric/min/max).
func TestNumericByteSlice(t *testing.T) {
	vd := MustValue([]byte("42"), "numeric && min:10")
	vd.Validate(context.Background())
	if vd.Fails() {
		t.Errorf(`[]byte("42") should be numeric and >=10, got %v`, vd.Errors().All())
	}
}

// TestMapEndToEnd covers the map path: builtins, boolean DSL, cross-field siblings, idempotency.
func TestMapEndToEnd(t *testing.T) {
	v := MustNew()

	t.Run("boolean DSL with builtins", func(t *testing.T) {
		vd := v.MustMap(
			map[string]any{"email": "a@b.com", "name": "alice"},
			map[string]string{
				"email": "required && email",
				"name":  "required && (alpha || in:bob,carol)",
			},
		)
		vd.Validate(context.Background())
		if vd.Fails() {
			t.Errorf("expected pass, got %v", vd.Errors().All())
		}
	})

	t.Run("failure collects errors", func(t *testing.T) {
		vd := v.MustMap(
			map[string]any{"email": "nope", "age": "abc"},
			map[string]string{
				"email": "required && email",
				"age":   "required && numeric",
			},
		)
		vd.Validate(context.Background())
		if !vd.Fails() {
			t.Fatal("expected failure")
		}
		if !vd.Errors().Has("email") || !vd.Errors().Has("age") {
			t.Errorf("expected errors for email and age, got %v", vd.Errors().All())
		}
	})

	t.Run("cross-field confirmed/same via siblings", func(t *testing.T) {
		ok := v.MustMap(
			map[string]any{"password": "secret", "password_confirmation": "secret"},
			map[string]string{"password": "required && confirmed"},
		)
		ok.Validate(context.Background())
		if ok.Fails() {
			t.Errorf("matching confirmation should pass: %v", ok.Errors().All())
		}

		bad := v.MustMap(
			map[string]any{"password": "secret", "password_confirmation": "different"},
			map[string]string{"password": "required && confirmed"},
		)
		bad.Validate(context.Background())
		if !bad.Fails() {
			t.Error("mismatched confirmation should fail")
		}
	})

	t.Run("cross-field same/gtfield", func(t *testing.T) {
		vd := v.MustMap(
			map[string]any{"min": 5, "max": 10},
			map[string]string{"max": "required && gtfield:min"},
		)
		vd.Validate(context.Background())
		if vd.Fails() {
			t.Errorf("10 > 5 should pass: %v", vd.Errors().All())
		}
	})

	t.Run("idempotent Validate", func(t *testing.T) {
		vd := v.MustMap(map[string]any{"a": ""}, map[string]string{"a": "required"})
		vd.Validate(context.Background())
		first := len(vd.Errors().All())
		vd.Validate(context.Background()) // must not double-add
		if len(vd.Errors().All()) != first {
			t.Error("Validate must be idempotent")
		}
	})
}

func TestJSONAndValues(t *testing.T) {
	v := MustNew()

	jv := v.MustJSON(`{"email":"a@b.com"}`, map[string]string{"email": "required && email"})
	jv.Validate(context.Background())
	if jv.Fails() {
		t.Errorf("JSON valid email should pass: %v", jv.Errors().All())
	}

	uv := v.MustValues(map[string][]string{"age": {"42"}}, map[string]string{"age": "required && numeric"})
	uv.Validate(context.Background())
	if uv.Fails() {
		t.Errorf("Values numeric should pass: %v", uv.Errors().All())
	}
}

// TestPackageHelpers covers the package-level wrappers (Default, Values) over the shared validator.
func TestPackageHelpers(t *testing.T) {
	d1, d2 := Default(), Default()
	if d1 == nil {
		t.Fatal("Default() must not be nil")
	}
	if d1 != d2 {
		t.Error("Default() should return the same shared validator across calls")
	}

	ctx := context.Background()
	ok := MustValues(url.Values{"age": {"42"}}, map[string]string{"age": "required && numeric"})
	ok.Validate(ctx)
	if ok.Fails() {
		t.Errorf("package Values should pass: %v", ok.Errors().All())
	}
	bad := MustValues(url.Values{"age": {"abc"}}, map[string]string{"age": "numeric"})
	bad.Validate(ctx)
	if !bad.Fails() {
		t.Error("package Values should fail on a non-numeric value")
	}
}
