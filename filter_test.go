package validator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestFiltersAppliedInValidate(t *testing.T) {
	v := MustNew()

	// trim+lower of "  ALICE  " yields "alice", which passes alpha && min:3
	vd := v.MustMap(map[string]any{"name": "  ALICE  "}, map[string]string{"name": "required && alpha && min:3"})
	if err := vd.AddFilters("name", "trim", "lower"); err != nil {
		t.Fatal(err)
	}
	vd.Validate(context.Background())
	if vd.Fails() {
		t.Errorf("filtered value should pass alpha (got errors %v)", vd.Errors().All())
	}
}

func TestFiltersAffectBind(t *testing.T) {
	type Form struct {
		Name string
	}
	vd := MustMap(map[string]any{"Name": "  Bob  "}, map[string]string{})
	if err := vd.AddFilters("Name", "trim", "upper"); err != nil {
		t.Fatal(err)
	}
	vd.Validate(context.Background())
	var f Form
	if err := vd.ValidateAs(context.Background(), &f); err != nil {
		t.Fatal(err)
	}
	if f.Name != "BOB" {
		t.Errorf("bind should use the filtered value, got %q", f.Name)
	}
}

func TestFilterIntConversion(t *testing.T) {
	type Form struct {
		Age int
	}
	vd := MustMap(map[string]any{"Age": "42"}, map[string]string{"Age": "numeric"})
	if err := vd.AddFilters("Age", "int"); err != nil {
		t.Fatal(err)
	}
	vd.Validate(context.Background())
	var f Form
	if err := vd.Bind(&f); err != nil {
		t.Fatal(err)
	}
	if f.Age != 42 {
		t.Errorf("int filter + bind should yield 42, got %d", f.Age)
	}
}

// Concurrent validation with per-field filters must not data-race.
func TestConcurrentFilters(t *testing.T) {
	v := MustNew(WithParallel(1))
	data := map[string]any{}
	sigs := map[string]string{}
	for i := range 64 {
		f := fmt.Sprintf("f%02d", i)
		data[f] = "  ABC  "
		sigs[f] = "required && alpha"
	}
	vd := v.MustMap(data, sigs)
	for i := range 64 {
		_ = vd.AddFilters(fmt.Sprintf("f%02d", i), "trim", "lower")
	}
	vd.Validate(context.Background())
	if vd.Fails() {
		t.Errorf("filtered values should pass: %v", vd.Errors().All())
	}
}

func TestRemoveFiltersEscapeAware(t *testing.T) {
	vd := MustMap(map[string]any{"x": "a"}, map[string]string{})
	if err := vd.AddFilters("x", `trim:\|`); err != nil {
		t.Fatal(err)
	}
	if err := vd.AddFilters("x", "lower"); err != nil {
		t.Fatal(err)
	}
	// removing lower must leave the escaped-arg trim filter intact
	if err := vd.RemoveFilters("x", "lower"); err != nil {
		t.Fatal(err)
	}
	got := vd.Filters()["x"]
	if got != `trim:\|` {
		t.Errorf("RemoveFilters corrupted the escaped-arg filter: got %q", got)
	}
}

type nilSliceFilter struct{}

// A filter returning a typed-nil must be unwrapped to canonical nil before rules/ValidateAs see it.
func TestFilteredTypedNilUnwrapped(t *testing.T) {
	v := MustNew(
		WithFilters(nilSliceFilter{}),
		WithRuleFunc("r5mustnil", func(f *Field) bool { return !f.Reflect().IsValid() }, "value must be canonical nil"),
	)
	vd := v.MustMap(map[string]any{"X": "input"}, map[string]string{"X": "r5mustnil"})
	if err := vd.AddFilters("X", "r5nilslice"); err != nil {
		t.Fatal(err)
	}
	vd.Validate(context.Background())
	if vd.Fails() {
		t.Errorf("a filter returning a typed-nil slice must be unwrapped to nil before rules see it, got %v", vd.Errors().All())
	}
}

// Rules()/Filters() return copies; mutating them cannot corrupt internal state.
func TestRulesFiltersAreCopies(t *testing.T) {
	vd := MustMap(map[string]any{"a": "x"}, map[string]string{"a": "required"})
	_ = vd.AddFilters("a", "trim")
	r := vd.Rules()
	r["a"] = "tampered"
	r["b"] = "injected"
	f := vd.Filters()
	f["a"] = "tampered"
	if vd.Rules()["a"] != "required" || vd.Rules()["b"] != "" {
		t.Errorf("Rules() must return a copy, internal state changed: %v", vd.Rules())
	}
	if vd.Filters()["a"] != "trim" {
		t.Errorf("Filters() must return a copy, internal state changed: %v", vd.Filters())
	}
}

// Bind writes RAW values; ValidateAs writes FILTERED values.
func TestBindRawValidateAsFiltered(t *testing.T) {
	type Form struct{ Name string }
	mk := func() *Validation {
		vd := MustMap(map[string]any{"Name": "  bob  "}, map[string]string{"Name": "required"})
		if err := vd.AddFilters("Name", "trim|upper"); err != nil {
			t.Fatal(err)
		}
		return vd
	}

	raw := mk()
	raw.Validate(context.Background())
	var rb Form
	if err := raw.Bind(&rb); err != nil {
		t.Fatal(err)
	}
	if rb.Name != "  bob  " {
		t.Errorf("Bind must write the RAW value even after Validate, got %q", rb.Name)
	}

	flt := mk()
	flt.Validate(context.Background())
	var fb Form
	if err := flt.ValidateAs(context.Background(), &fb); err != nil {
		t.Fatal(err)
	}
	if fb.Name != "BOB" {
		t.Errorf("ValidateAs must write the FILTERED value, got %q", fb.Name)
	}
}

// Filter arguments may contain separators via backslash escaping.
func TestFilterArgEscaping(t *testing.T) {
	vd := MustMap(map[string]any{"S": ",a,b,"}, map[string]string{"S": "required"})
	if err := vd.AddFilters("S", `trim:\,`); err != nil {
		t.Fatal(err)
	}
	vd.Validate(context.Background())
	var out struct{ S string }
	if err := vd.ValidateAs(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	if out.S != "a,b" {
		t.Errorf(`trim:\, should trim commas, got %q`, out.S)
	}
}

// A trailing backslash in one chunk must not swallow the next filter across the join boundary.
func TestAddFiltersEscapedJoin(t *testing.T) {
	vd := MustMap(map[string]any{"S": `\HELLO\`}, map[string]string{"S": "required"})
	if err := vd.AddFilters("S", `trim:\`, "lower"); err != nil {
		t.Fatal(err)
	}
	vd.Validate(context.Background())
	var out struct{ S string }
	if err := vd.ValidateAs(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	if out.S != "hello" {
		t.Errorf("both filters must run despite a trailing backslash; got %q want hello", out.S)
	}
}

// AddRules/AddFilters are atomic: a later bad arg leaves the field unchanged.
func TestAddRulesFiltersAtomic(t *testing.T) {
	v := MustNew()
	vd := v.MustMap(map[string]any{}, map[string]string{})
	if err := vd.AddRules("x", "alpha", "nosuchrule", "numeric"); err == nil {
		t.Error("AddRules with an unknown rule should error")
	}
	if vd.Rules()["x"] != "" {
		t.Errorf("a failed AddRules must leave the field unchanged, got %q", vd.Rules()["x"])
	}
	if err := vd.AddFilters("y", "trim", "nosuchfilter"); err == nil {
		t.Error("AddFilters with an unknown filter should error")
	}
	if vd.Filters()["y"] != "" {
		t.Errorf("a failed AddFilters must leave the field unchanged, got %q", vd.Filters()["y"])
	}
}

// ValidateAs on a diving field with a scalar filter binds the RAW collection, not its stringified form.
func TestValidateAsDivingFieldFilter(t *testing.T) {
	type T struct{ Tags []string }
	vd := MustMap(map[string]any{"Tags": []any{"a", "b"}}, map[string]string{"Tags": "dive && alpha"})
	if err := vd.AddFilters("Tags", "trim"); err != nil {
		t.Fatal(err)
	}
	vd.Validate(context.Background())
	if vd.Fails() {
		t.Fatalf("dive validation should pass: %v", vd.Errors().All())
	}
	var out T
	if err := vd.ValidateAs(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Tags) != 2 || out.Tags[0] != "a" || out.Tags[1] != "b" {
		t.Errorf("ValidateAs on a diving field with a scalar filter must bind the raw collection, got %v", out.Tags)
	}
}

// A failing filter is a validation failure, never a silent raw value.
func TestFilterFailureSurfaces(t *testing.T) {
	vd := MustMap(map[string]any{"age": "abc"}, map[string]string{"age": "numeric"})
	if err := vd.AddFilters("age", "int"); err != nil {
		t.Fatal(err)
	}
	vd.Validate(context.Background())
	if !vd.Errors().Has("age") {
		t.Fatalf("a failing int filter must surface as a field error, got %v", vd.Errors().All())
	}
	if msg := vd.Errors().OneFor("age"); !strings.Contains(msg, "filter") {
		t.Errorf("the message should name the failing filter, got %q", msg)
	}

	// ValidateAs refuses after a filter failure.
	var out struct{ Age int }
	if err := vd.ValidateAs(context.Background(), &out); err == nil {
		t.Error("ValidateAs must refuse when a filter failed")
	}

	// the successful case still binds the filtered value
	ok := MustMap(map[string]any{"age": " 42 "}, map[string]string{"age": "numeric"})
	if err := ok.AddFilters("age", "trim"); err != nil {
		t.Fatal(err)
	}
	ok.Validate(context.Background())
	if ok.Fails() {
		t.Fatalf("trim + numeric should pass, got %v", ok.Errors().All())
	}
}

func TestNewRejectsInvalidSignatures(t *testing.T) {
	for _, sig := range []string{"", "   ", "has space", "a&&b", "dive", "9lead", "x:y"} {
		if _, err := New(WithRuleFunc(sig, func(*Field) bool { return true }, "m")); !errors.Is(err, ErrInvalidSignature) {
			t.Errorf("signature %q error = %v", sig, err)
		}
	}
	if _, err := New(WithRuleFunc("my_rule2", func(*Field) bool { return true }, "m")); err != nil {
		t.Fatal(err)
	}
}

// The package-level Valid mirrors Validator.Valid.
func TestPackageLevelValid(t *testing.T) {
	type T struct {
		N string `validate:"notblank"`
	}
	if valid, err := Valid(t.Context(), T{}); err != nil || valid {
		t.Error("Valid must fail a blank notblank field")
	}
	if valid, err := Valid(t.Context(), T{N: "x"}); err != nil || !valid {
		t.Error("Valid should pass")
	}
	if _, err := Valid(t.Context(), map[string]any{"a": 1}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("map input error = %v, want ErrInvalidInput", err)
	}
}

// An absent value (nil pointer, missing key) has nothing to filter: running the
// chain would materialize a zero value ("" from trim(nil)) that ValidateAs then
// writes over the target's existing data — a silent PATCH overwrite.
func TestFilterSkipsAbsentValue(t *testing.T) {
	type Patch struct {
		Name *string `validate:"sometimes && required && min:3"`
	}
	v := MustNew()
	vd := v.MustStruct(&Patch{}) // Name absent
	if err := vd.AddFilters("Name", "trim"); err != nil {
		t.Fatal(err)
	}
	vd.Validate(context.Background())
	if vd.Fails() {
		t.Fatalf("absent field must skip: %v", vd.Errors().All())
	}
	prev := "keep-me"
	dst := Patch{Name: &prev}
	if err := vd.ValidateAs(context.Background(), &dst); err != nil {
		t.Fatal(err)
	}
	if dst.Name == nil || *dst.Name != "keep-me" {
		t.Errorf("ValidateAs must not materialize an absent filtered field, got %v", dst.Name)
	}

	// A present value still flows through the filter into ValidateAs.
	val := "  padded  "
	pv := v.MustStruct(&Patch{Name: &val})
	_ = pv.AddFilters("Name", "trim")
	pv.Validate(context.Background())
	var pdst Patch
	if err := pv.ValidateAs(context.Background(), &pdst); err != nil {
		t.Fatal(err)
	}
	if pdst.Name == nil || *pdst.Name != "padded" {
		t.Errorf("present value must be filtered and bound, got %v", pdst.Name)
	}
}
