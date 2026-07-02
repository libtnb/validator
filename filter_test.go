package validator

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestFiltersAppliedInValidate(t *testing.T) {
	v := NewValidator()

	// trim+lower of "  ALICE  " yields "alice", which passes alpha && min:3
	vd := v.Map(map[string]any{"name": "  ALICE  "}, map[string]string{"name": "required && alpha && min:3"})
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
	vd := Map(map[string]any{"Name": "  Bob  "}, map[string]string{})
	if err := vd.AddFilters("Name", "trim", "upper"); err != nil {
		t.Fatal(err)
	}
	vd.Validate(context.Background())
	var f Form
	if err := vd.SafeBind(&f); err != nil {
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
	vd := Map(map[string]any{"Age": "42"}, map[string]string{"Age": "numeric"})
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
	v := NewValidator(WithParallel(1))
	data := map[string]any{}
	sigs := map[string]string{}
	for i := range 64 {
		f := fmt.Sprintf("f%02d", i)
		data[f] = "  ABC  "
		sigs[f] = "required && alpha"
	}
	vd := v.Map(data, sigs)
	for i := range 64 {
		_ = vd.AddFilters(fmt.Sprintf("f%02d", i), "trim", "lower")
	}
	vd.Validate(context.Background())
	if vd.Fails() {
		t.Errorf("filtered values should pass: %v", vd.Errors().All())
	}
}

func TestRemoveFiltersEscapeAware(t *testing.T) {
	vd := Map(map[string]any{"x": "a"}, map[string]string{})
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

// A filter returning a typed-nil must be unwrapped to canonical nil before rules/SafeBind see it.
func TestFilteredTypedNilUnwrapped(t *testing.T) {
	v := NewValidator()
	v.RegisterFilter(nilSliceFilter{})
	v.RegisterFunc("r5mustnil", func(f Field) bool { return !f.Val().IsValid() }, "value must be canonical nil")
	vd := v.Map(map[string]any{"X": "input"}, map[string]string{"X": "r5mustnil"})
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
	vd := Map(map[string]any{"a": "x"}, map[string]string{"a": "required"})
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

// Bind writes RAW values; SafeBind writes FILTERED values.
func TestBindRawSafeBindFiltered(t *testing.T) {
	type Form struct{ Name string }
	mk := func() Validation {
		vd := Map(map[string]any{"Name": "  bob  "}, map[string]string{"Name": "required"})
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
	if err := flt.SafeBind(&fb); err != nil {
		t.Fatal(err)
	}
	if fb.Name != "BOB" {
		t.Errorf("SafeBind must write the FILTERED value, got %q", fb.Name)
	}
}

// Filter arguments may contain separators via backslash escaping.
func TestFilterArgEscaping(t *testing.T) {
	vd := Map(map[string]any{"S": ",a,b,"}, map[string]string{"S": "required"})
	if err := vd.AddFilters("S", `trim:\,`); err != nil {
		t.Fatal(err)
	}
	vd.Validate(context.Background())
	var out struct{ S string }
	if err := vd.SafeBind(&out); err != nil {
		t.Fatal(err)
	}
	if out.S != "a,b" {
		t.Errorf(`trim:\, should trim commas, got %q`, out.S)
	}
}

// A trailing backslash in one chunk must not swallow the next filter across the join boundary.
func TestAddFiltersEscapedJoin(t *testing.T) {
	vd := Map(map[string]any{"S": `\HELLO\`}, map[string]string{"S": "required"})
	if err := vd.AddFilters("S", `trim:\`, "lower"); err != nil {
		t.Fatal(err)
	}
	vd.Validate(context.Background())
	var out struct{ S string }
	if err := vd.SafeBind(&out); err != nil {
		t.Fatal(err)
	}
	if out.S != "hello" {
		t.Errorf("both filters must run despite a trailing backslash; got %q want hello", out.S)
	}
}

// AddRules/AddFilters are atomic: a later bad arg leaves the field unchanged.
func TestAddRulesFiltersAtomic(t *testing.T) {
	v := NewValidator()
	vd := v.Map(map[string]any{}, map[string]string{})
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

// SafeBind of a diving field with a scalar filter binds the RAW collection, not its stringified form.
func TestSafeBindDivingFieldFilter(t *testing.T) {
	type T struct{ Tags []string }
	vd := Map(map[string]any{"Tags": []any{"a", "b"}}, map[string]string{"Tags": "dive && alpha"})
	if err := vd.AddFilters("Tags", "trim"); err != nil {
		t.Fatal(err)
	}
	vd.Validate(context.Background())
	if vd.Fails() {
		t.Fatalf("dive validation should pass: %v", vd.Errors().All())
	}
	var out T
	if err := vd.SafeBind(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.Tags) != 2 || out.Tags[0] != "a" || out.Tags[1] != "b" {
		t.Errorf("SafeBind of a diving field with a scalar filter must bind the raw collection, got %v", out.Tags)
	}
}

// A failing filter is a validation failure, never a silent raw value.
func TestFilterFailureSurfaces(t *testing.T) {
	vd := Map(map[string]any{"age": "abc"}, map[string]string{"age": "numeric"})
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

	// SafeBind refuses after a filter failure
	var out struct{ Age int }
	if err := vd.SafeBind(&out); err == nil {
		t.Error("SafeBind must refuse when a filter failed")
	}

	// the successful case still binds the filtered value
	ok := Map(map[string]any{"age": " 42 "}, map[string]string{"age": "numeric"})
	if err := ok.AddFilters("age", "trim"); err != nil {
		t.Fatal(err)
	}
	ok.Validate(context.Background())
	if ok.Fails() {
		t.Fatalf("trim + numeric should pass, got %v", ok.Errors().All())
	}
}

// Register* panics on signatures the DSL could never invoke.
func TestRegisterInvalidSignaturePanics(t *testing.T) {
	v := NewValidator()
	for _, sig := range []string{"", "   ", "has space", "a&&b", "dive", "9lead", "x:y"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("RegisterFunc(%q) must panic on an uninvokable signature", sig)
				}
			}()
			v.RegisterFunc(sig, func(Field) bool { return true }, "m")
		}()
	}
	// a legal signature registers fine
	v.RegisterFunc("my_rule2", func(Field) bool { return true }, "m")
}

// The package-level Valid mirrors Validator.Valid.
func TestPackageLevelValid(t *testing.T) {
	type T struct {
		N string `validate:"notblank"`
	}
	if Valid(T{}) {
		t.Error("Valid must fail a blank notblank field")
	}
	if !Valid(T{N: "x"}) {
		t.Error("Valid should pass")
	}
	if !Valid(map[string]any{"a": 1}) {
		t.Error("data with no rules is vacuously valid")
	}
}
