package validator

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"
)

func TestDivingNestedStruct(t *testing.T) {
	type Address struct {
		City string `validate:"required"`
		Zip  string `validate:"required && digits:5"`
	}
	type User struct {
		Name    string  `validate:"required"`
		Address Address // auto-recursed
	}
	v := NewValidator()

	t.Run("nested valid", func(t *testing.T) {
		vd := v.Struct(User{Name: "alice", Address: Address{City: "NYC", Zip: "10001"}})
		vd.Validate(context.Background())
		if vd.Fails() {
			t.Errorf("valid nested struct should pass: %v", vd.Errors().All())
		}
	})

	t.Run("nested failure uses dotted key", func(t *testing.T) {
		vd := v.Struct(User{Name: "alice", Address: Address{City: "NYC", Zip: "bad"}})
		vd.Validate(context.Background())
		if !vd.Fails() {
			t.Fatal("invalid nested zip should fail")
		}
		if !vd.Errors().Has("Address.Zip") {
			t.Errorf("expected error keyed Address.Zip, got %v", vd.Errors().All())
		}
	})
}

func TestDivingNestedPointer(t *testing.T) {
	type Profile struct {
		Bio string `validate:"required"`
	}
	type Account struct {
		Profile *Profile
	}
	v := NewValidator()

	// nil pointer: nested field absent, required fails
	vd := v.Struct(Account{Profile: nil})
	vd.Validate(context.Background())
	if !vd.Fails() {
		t.Errorf("nil nested pointer with required field should fail, got %v", vd.Errors().All())
	}

	ok := v.Struct(Account{Profile: &Profile{Bio: "hi"}})
	ok.Validate(context.Background())
	if ok.Fails() {
		t.Errorf("valid nested pointer should pass: %v", ok.Errors().All())
	}
}

// TestDivingSelfReferential: plan builder must terminate on recursive types.
func TestDivingSelfReferential(t *testing.T) {
	type Node struct {
		Name string `validate:"required"`
		Next *Node
	}
	v := NewValidator()
	vd := v.Struct(Node{Name: "root"})
	vd.Validate(context.Background())
	if vd.Fails() {
		t.Errorf("self-referential type should validate: %v", vd.Errors().All())
	}
}

func TestMultipleDiveFailsClearly(t *testing.T) {
	v := NewValidator()
	vd := v.Map(
		map[string]any{"m": []any{"a@b.com"}},
		map[string]string{"m": "dive && dive && email"},
	)
	vd.Validate(context.Background())
	if !vd.Errors().Has("m") {
		t.Errorf("multiple dive should report one field-level error on 'm', got %v", vd.Errors().All())
	}
	if vd.Errors().Has("m[0]") {
		t.Errorf("multiple dive must not produce per-element 'unknown rule dive': %v", vd.Errors().All())
	}
}

func TestDiveContainerAddRule(t *testing.T) {
	v := NewValidator()
	vd := v.Map(map[string]any{}, map[string]string{"tags": "dive && alpha"})
	if err := vd.AddRules("tags", "required"); err != nil {
		t.Fatal(err)
	}
	vd.Validate(context.Background())
	// required must apply to the container, not be demoted to elements
	if !vd.Errors().Has("tags") {
		t.Errorf("AddRule(required) should apply to the diving container, got %v", vd.Errors().All())
	}
}

// A non-empty non-collection under dive must fail closed; empty/absent still skip via omitempty.
func TestDiveNonCollectionFailsClosed(t *testing.T) {
	bad := Map(map[string]any{"tags": "123"}, map[string]string{"tags": "required && dive && alpha"})
	bad.Validate(context.Background())
	if !bad.Errors().Has("tags") {
		t.Errorf("a non-empty scalar under dive must fail (it is not a collection), got %v", bad.Errors().All())
	}

	empty := Map(map[string]any{"tags": ""}, map[string]string{"tags": "dive && alpha"})
	empty.Validate(context.Background())
	if empty.Fails() {
		t.Errorf("an empty value under dive must be skipped (omitempty), got %v", empty.Errors().All())
	}

	absent := Map(map[string]any{}, map[string]string{"tags": "dive && alpha"})
	absent.Validate(context.Background())
	if absent.Fails() {
		t.Errorf("an absent field under dive must be skipped, got %v", absent.Errors().All())
	}

	// a real collection still validates element-wise
	ok := Map(map[string]any{"tags": []any{"abc", "def"}}, map[string]string{"tags": "dive && alpha"})
	ok.Validate(context.Background())
	if ok.Fails() {
		t.Errorf("a valid collection under dive must pass, got %v", ok.Errors().All())
	}
	ng := Map(map[string]any{"tags": []any{"abc", "1"}}, map[string]string{"tags": "dive && alpha"})
	ng.Validate(context.Background())
	if !ng.Errors().Has("tags[1]") {
		t.Errorf("an invalid element under dive must still fail per-element, got %v", ng.Errors().All())
	}
}

// evalDive's container deref must be bounded so a recursive pointer type can't loop forever.
func TestDiveRecursivePointerNoHang(t *testing.T) {
	completesWithin(t, "evalDive recursive pointer", func() {
		type P *P
		var p P
		p = &p
		Var(p, "dive && required").Validate(context.Background())
	})
}

// A map dive over indistinguishable keys (distinct NaN floats) must produce deterministic error keys.
func TestDiveMapNaNDeterministic(t *testing.T) {
	m := map[float64]any{math.NaN(): "1", math.NaN(): "2"}
	var want map[string]map[string]string
	for i := range 25 {
		v := Map(map[string]any{"m": m}, map[string]string{"m": "dive && alpha"})
		v.Validate(context.Background())
		got := v.Errors().All()
		if len(got) != 2 {
			t.Fatalf("expected 2 distinct error keys for 2 NaN entries, got %v", got)
		}
		if want == nil {
			want = got
			continue
		}
		for k, inner := range want {
			gi, ok := got[k]
			if !ok {
				t.Fatalf("nondeterministic dive-map keys: run %d missing %q (want %v, got %v)", i, k, want, got)
			}
			for rule, msg := range inner {
				if gi[rule] != msg {
					t.Fatalf("nondeterministic message for %q.%q: %q vs %q", k, rule, msg, gi[rule])
				}
			}
		}
	}
}

// RemoveRules must never remove the reserved structural 'dive' separator.
func TestRemoveRulesKeepsDive(t *testing.T) {
	vd := Map(map[string]any{"x": []any{"a"}}, map[string]string{"x": "required && dive && alpha"})
	if err := vd.RemoveRules("x", "dive"); err != nil {
		t.Fatal(err)
	}
	if got := vd.Rules()["x"]; !strings.Contains(got, "dive") {
		t.Errorf("RemoveRules must keep the structural dive separator, got %q", got)
	}
	// a genuine element rule is still removable
	if err := vd.RemoveRules("x", "alpha"); err != nil {
		t.Fatal(err)
	}
	if got := vd.Rules()["x"]; !strings.Contains(got, "dive") || strings.Contains(got, "alpha") {
		t.Errorf("removing alpha should keep dive and drop alpha, got %q", got)
	}
}

// Dive-map keys must stay deterministic when distinct keys hold same-%v values of different types.
func TestDiveMapValueTypeTiebreak(t *testing.T) {
	t0 := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	var want map[string]map[string]string
	for i := range 25 {
		m := map[float64]any{}
		m[math.NaN()] = t0          // time.Time passes date
		m[math.NaN()] = t0.String() // its string form fails date
		vd := Map(map[string]any{"m": m}, map[string]string{"m": "dive && date"})
		vd.Validate(context.Background())
		got := vd.Errors().All()
		if len(got) != 1 {
			t.Fatalf("expected exactly 1 dive error (the string value fails date), got %v", got)
		}
		if want == nil {
			want = got
			continue
		}
		for k := range want {
			if _, ok := got[k]; !ok {
				t.Fatalf("nondeterministic dive-map error key on run %d: got %v, want key %q", i, got, k)
			}
		}
	}
}

// A scalar filter on a diving field must not stringify the collection the container rule sees.
func TestDiveContainerUsesRaw(t *testing.T) {
	sv := NewValidator(WithStrictRequired())
	vd := sv.Map(map[string]any{"tags": []any{}}, map[string]string{"tags": "required && dive && alpha"})
	if err := vd.AddFilters("tags", "trim"); err != nil {
		t.Fatal(err)
	}
	vd.Validate(context.Background())
	if !vd.Errors().Has("tags") {
		t.Error("required on an empty diving slice must fail even with a scalar filter configured (container must see the raw slice, not \"[]\")")
	}
}

// dive must dereference a pointer container (*[]T) so elements are validated.
func TestDivePointerContainer(t *testing.T) {
	emails := []any{"a@b.com", "nope"}
	vd := Map(map[string]any{"emails": &emails}, map[string]string{"emails": "dive && email"})
	vd.Validate(context.Background())
	if !vd.Errors().Has("emails[1]") {
		t.Errorf("dive should deref a *[]T container and validate elements, got %v", vd.Errors().All())
	}
}

// a nested dive (even parenthesized) yields one field-level error, never per-element.
func TestNestedDiveSingleError(t *testing.T) {
	vd := Map(map[string]any{"m": []any{"x"}}, map[string]string{"m": "dive && (dive && email)"})
	vd.Validate(context.Background())
	if !vd.Errors().Has("m") {
		t.Errorf("a parenthesized nested dive should report one field-level error, got %v", vd.Errors().All())
	}
	if vd.Errors().Has("m[0]") {
		t.Errorf("must not leak per-element unknown-rule errors, got %v", vd.Errors().All())
	}
}

// two distinct map keys formatting identically get distinct error keys.
func TestMapDiveKeyCollision(t *testing.T) {
	vd := Map(map[string]any{"scores": map[any]any{1: "9", "1": "8"}}, map[string]string{"scores": "dive && alpha"})
	vd.Validate(context.Background())
	n := 0
	for k := range vd.Errors().All() {
		if strings.HasPrefix(k, "scores[") {
			n++
		}
	}
	if n != 2 {
		t.Errorf("two distinct keys formatting as \"1\" must yield two error keys, got %d: %v", n, vd.Errors().All())
	}
}

// dive-element messages resolve the base alias and field override, keeping the [index].
func TestDiveMessageBaseAliasAndOverride(t *testing.T) {
	v := NewValidator(WithAttributes(map[string]string{"tags": "Labels"}))
	vd := v.Map(map[string]any{"tags": []any{"ok", "9bad"}}, map[string]string{"tags": "dive && alpha"})
	vd.Validate(context.Background())
	if msg := vd.Errors().OneFor("tags[1]"); !strings.Contains(msg, "Labels[1]") {
		t.Errorf("dive element message should use the base alias + index (Labels[1]), got %q", msg)
	}

	v2 := NewValidator(WithMessages(map[string]string{"tags.alpha": "element is bad"}))
	vd2 := v2.Map(map[string]any{"tags": []any{"9bad"}}, map[string]string{"tags": "dive && alpha"})
	vd2.Validate(context.Background())
	if msg := vd2.Errors().OneFor("tags[0]"); msg != "element is bad" {
		t.Errorf("a field-level override (tags.alpha) should apply to dive elements, got %q", msg)
	}
}

// a NaN map key must not panic (it would escape Validate, fatal under WithParallel).
func TestDiveMapNaNKeyNoPanic(t *testing.T) {
	nan := func() float64 { z := 0.0; return z / z }() // not a const: avoids const-div error
	for _, par := range []int{0, 1} {
		var opts []Option
		if par > 0 {
			opts = append(opts, WithParallel(par))
		}
		v := NewValidator(opts...)
		vd := v.Map(map[string]any{"scores": map[float64]string{nan: "x", 1.5: "y"}},
			map[string]string{"scores": "dive && numeric"})
		vd.Validate(context.Background())
		_ = vd.Fails()
	}
}

// a base field-level override must beat a rule-level override for a dive element.
func TestDiveMessageFieldBeatsRule(t *testing.T) {
	v := NewValidator(WithMessages(map[string]string{"tags.alpha": "BASE", "alpha": "RULE"}))
	vd := v.Map(map[string]any{"tags": []any{"9bad"}}, map[string]string{"tags": "dive && alpha"})
	vd.Validate(context.Background())
	if got := vd.Errors().OneFor("tags[0]"); got != "BASE" {
		t.Errorf("base field-level override must beat rule-level for a dive element, got %q", got)
	}
}

// a map dive key containing '[' still resolves the base override.
func TestDiveMapKeyWithBracket(t *testing.T) {
	v := NewValidator(WithMessages(map[string]string{"m.alpha": "OVERRIDE"}))
	vd := v.Map(map[string]any{"m": map[string]any{"a[b": "9"}}, map[string]string{"m": "dive && alpha"})
	vd.Validate(context.Background())
	if got := vd.Errors().OneFor("m[a[b]"); got != "OVERRIDE" {
		t.Errorf("base override should apply despite '[' in the key, got %q", got)
	}
}

// no dive-element errors are dropped when a real key mimics a synthesized ordinal label.
func TestDiveMapNoDropOnMimicKey(t *testing.T) {
	nan := math.NaN()
	m := map[any]any{}
	m[nan] = "9a" // two NaN keys coexist (NaN != NaN)
	m[nan] = "9b"
	m["NaN|float64"] = "9c" // mimics the synthesized label
	vd := Map(map[string]any{"m": m}, map[string]string{"m": "dive && alpha"})
	vd.Validate(context.Background())
	n := 0
	for k := range vd.Errors().All() {
		if strings.HasPrefix(k, "m[") {
			n++
		}
	}
	if n != 3 {
		t.Errorf("3 distinct failing elements must yield 3 error keys (no drop), got %d: %v", n, vd.Errors().All())
	}
}

// distinct same-%v keys of different types get distinct, type-labeled keys.
func TestDiveMapDistinctTypes(t *testing.T) {
	m := map[any]any{int(1): "9a", int64(1): "9b", "1": "9c"}
	vd := Map(map[string]any{"m": m}, map[string]string{"m": "dive && alpha"})
	vd.Validate(context.Background())
	n := 0
	for k := range vd.Errors().All() {
		if strings.HasPrefix(k, "m[") {
			n++
		}
	}
	if n != 3 {
		t.Errorf(`int(1), int64(1) and "1" must yield 3 distinct error keys, got %d: %v`, n, vd.Errors().All())
	}
}

// dive dereferences pointer elements like top-level sources.
func TestDivePointerElements(t *testing.T) {
	a, b := 5, 3
	ok := Map(map[string]any{"nums": []*int{&a, &b}}, map[string]string{"nums": "dive && numeric && gte:1"})
	ok.Validate(context.Background())
	if ok.Fails() {
		t.Errorf("dive should deref *int elements, got %v", ok.Errors().All())
	}
	bad := Map(map[string]any{"nums": []*int{&a}}, map[string]string{"nums": "dive && gte:100"})
	bad.Validate(context.Background())
	if !bad.Errors().Has("nums[0]") {
		t.Error("dive element 5 should fail gte:100 after deref")
	}
}

// Cross-field rules inside dive must resolve siblings identically on the fast
// (Valid) and diag (Validate) paths: the element's cross-field identity is the
// container name, so a sibling matching the collection's last path segment
// resolves the same (never the dived collection itself) on both.
func TestDiveCrossFieldFastDiagParity(t *testing.T) {
	type Inner struct {
		Items []string `validate:"dive && same:Items"`
	}
	type Outer struct {
		Items string
		In    Inner
	}
	v := NewValidator()
	for _, c := range []struct {
		name string
		data Outer
	}{
		{"element equals top-level Items", Outer{Items: "a", In: Inner{Items: []string{"a"}}}},
		{"element differs from top-level Items", Outer{Items: "a", In: Inner{Items: []string{"z"}}}},
	} {
		fast := v.Valid(c.data)
		vd := v.Struct(c.data)
		vd.Validate(context.Background())
		if fast == vd.Fails() {
			t.Errorf("%s: Valid()=%v but Validate().Fails()=%v — dive cross-field verdicts must agree", c.name, fast, vd.Fails())
		}
	}
}

// confirmed inside dive derives the confirmation from the container name on
// both paths — never from the bracketed element key.
func TestDiveConfirmedUsesContainerName(t *testing.T) {
	type S struct {
		Pw              []string `validate:"dive && confirmed"`
		Pw_confirmation string
	}
	v := NewValidator()
	match := S{Pw: []string{"x"}, Pw_confirmation: "x"}
	if !v.Valid(match) {
		t.Error("Valid: matching confirmation should pass")
	}
	vd := v.Struct(match)
	vd.Validate(context.Background())
	if vd.Fails() {
		t.Errorf("Validate: matching confirmation should pass, got %v", vd.Errors().All())
	}

	bad := S{Pw: []string{"x"}, Pw_confirmation: "other"}
	if v.Valid(bad) {
		t.Error("Valid: mismatched confirmation should fail")
	}
	bd := v.Struct(bad)
	bd.Validate(context.Background())
	if !bd.Fails() {
		t.Error("Validate: mismatched confirmation should fail")
	}

	// an internal element-label-shaped flat key must not satisfy confirmed
	md := v.Map(
		map[string]any{"pw": []string{"x"}, "pw[0]_confirmation": "x", "pw_confirmation": "DIFFERENT"},
		map[string]string{"pw": "dive && confirmed"},
	)
	md.Validate(context.Background())
	if !md.Fails() {
		t.Error("a flat key shaped like an internal element label must not satisfy confirmed")
	}
}

// A trailing dive with no element rules is a fail-fast expression error.
func TestDanglingDiveRejected(t *testing.T) {
	vd := Map(map[string]any{"tags": []string{"a"}}, map[string]string{"tags": "min:1 && dive"})
	vd.Validate(context.Background())
	if !vd.Errors().Has("tags") {
		t.Error("a trailing 'dive' must be a field-level expression error")
	}
	av := Map(map[string]any{"tags": []string{"a"}}, map[string]string{})
	if err := av.AddRules("tags", "min:1 && dive"); err == nil {
		t.Error("AddRules must reject a trailing 'dive'")
	}
}
