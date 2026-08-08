package validator

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/libtnb/validator/conv"
)

func validResult(t *testing.T, validator *Validator, data any) bool {
	t.Helper()
	valid, err := validator.Valid(t.Context(), data)
	if err != nil {
		t.Fatalf("Valid error: %T %#v (%q)", err, err, err.Error())
	}
	return valid
}

func TestErrorsReturnCopy(t *testing.T) {
	vd := MustMap(map[string]any{}, map[string]string{"a": "required"})
	vd.Validate(context.Background())
	all := vd.Errors().All()
	delete(all, "a")
	if !vd.Errors().Has("a") {
		t.Error("mutating the All() result must not corrupt internal error state")
	}
	g := vd.Errors().Messages("a")
	g["x"] = "y"
	if len(vd.Errors().Messages("a")) != 1 {
		t.Error("mutating the Messages() result must not corrupt internal error state")
	}
}

func TestPanicRecovered(t *testing.T) {
	v := MustNew(WithoutBuiltinRules(),
		WithRuleFunc("boom", func(*Field) bool { panic("boom") }, "m"))
	vd := v.MustMap(map[string]any{"a": "x"}, map[string]string{"a": "boom"})
	err := vd.Validate(context.Background())
	if !errors.Is(err, ErrRulePanic) {
		t.Fatalf("error = %v, want ErrRulePanic", err)
	}
	if len(vd.Errors().Items()) != 0 {
		t.Fatalf("rule panic became field errors: %v", vd.Errors().Items())
	}
}

type crashStringer struct{ p *int }

func (c crashStringer) String() string { return strings.Repeat("x", *c.p) } // nil deref

// TestStringerPanicContained: a user type whose String() panics must never
// crash through a public entry point — attach-time and filter-time value
// stringification is as guarded as rule evaluation.
func TestStringerPanicContained(t *testing.T) {
	v := MustNew()
	data := map[string]crashStringer{"a": {}}

	mustNotPanic := func(name string, fn func()) {
		t.Helper()
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("%s: panic escaped: %v", name, r)
			}
		}()
		fn()
	}

	mustNotPanic("Valid(map with panicking value Stringer)", func() { _, _ = v.Valid(t.Context(), data) })
	mustNotPanic("Valid(map with panicking key Stringer)", func() {
		_, _ = v.Valid(t.Context(), map[crashStringer]string{{}: "x"})
	})
	mustNotPanic("Struct", func() { _, _ = v.Struct(data) })
	mustNotPanic("Map+Validate", func() { v.MustMap(data, nil).Validate(context.Background()) })
	mustNotPanic("prepareFilters via dotted filter", func() {
		vd := v.MustMap(map[string]any{"outer": map[string]crashStringer{"b": {}}}, map[string]string{"outer.b": "alpha"})
		if err := vd.AddFilters("outer.b", "trim"); err != nil {
			t.Fatal(err)
		}
		vd.Validate(context.Background())
	})

	// dive over a map with panicking value Stringers: fast and diag must agree.
	type diveHolder struct {
		M map[string]crashStringer `validate:"dive && alpha"`
	}
	dh := diveHolder{M: map[string]crashStringer{"k": {}}}
	var fastOK, diagFails bool
	mustNotPanic("Valid(dive map)", func() { fastOK, _ = v.Valid(t.Context(), dh) })
	mustNotPanic("Validate(dive map)", func() {
		vd := v.MustStruct(dh)
		vd.Validate(context.Background())
		diagFails = vd.Fails()
	})
	if fastOK == diagFails {
		t.Errorf("Valid()=%v but Validate().Fails()=%v — fast/diag verdicts must agree", fastOK, diagFails)
	}
}

// nestedBomb panics WITH a value whose own String() panics: fmt re-escapes
// such a nested method panic, so every engine stringification and every
// recovered-panic formatter must contain it.
type nestedBomb struct{ X int }

func (nestedBomb) String() string { panic(nestedBomb{}) }

type bombHolder struct{ B nestedBomb }

func TestNestedStringerPanicContained(t *testing.T) {
	v := MustNew()
	mustNotPanic := func(name string, fn func()) {
		t.Helper()
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("%s: panic escaped: %v", name, r)
			}
		}()
		fn()
	}

	// container-nested bomb through attach-time stringification
	mustNotPanic("Valid(map[string]bombHolder)", func() { _, _ = v.Valid(t.Context(), map[string]bombHolder{"k": {}}) })
	mustNotPanic("Valid(map[string][]nestedBomb)", func() { _, _ = v.Valid(t.Context(), map[string][]nestedBomb{"k": {{}}}) })
	mustNotPanic("MustMap(map[string]bombHolder)+Validate", func() {
		v.MustMap(map[string]bombHolder{"k": {}}, nil).Validate(context.Background())
	})

	// a rule that panics WITH a bomb: the recovery's own message formatting
	// must not re-escape
	rv := MustNew(WithoutBuiltinRules(),
		WithRuleFunc("boom", func(*Field) bool { panic(nestedBomb{}) }, "m"))
	mustNotPanic("rule panicking with a bomb value", func() {
		vd := rv.MustMap(map[string]any{"a": "x"}, map[string]string{"a": "boom"})
		vd.Validate(context.Background())
		if !vd.Fails() {
			t.Error("the panicking rule must degrade to a field error")
		}
	})

	// a filter that panics with a bomb
	fv := MustNew(WithFilters(bombFilter{}))
	mustNotPanic("filter panicking with a bomb value", func() {
		vd := fv.MustMap(map[string]any{"a": "x"}, map[string]string{"a": "alpha"})
		if err := vd.AddFilters("a", "bombf"); err != nil {
			t.Fatal(err)
		}
		vd.Validate(context.Background())
		if !vd.Fails() {
			t.Error("the panicking filter must surface as a field error")
		}
	})
}

type bombFilter struct{}

func (bombFilter) Signature() string                  { return "bombf" }
func (bombFilter) Handle(any, ...string) (any, error) { panic(nestedBomb{}) }

func TestAddRuleParenthesized(t *testing.T) {
	v := MustNew()
	// an appended rule must parenthesize the whole expression, not be swallowed by a top-level ||.
	vd := v.MustMap(map[string]any{}, map[string]string{"a": "alpha || numeric"})
	_ = vd.AddRules("a", "required")
	vd.Validate(context.Background())
	if !vd.Errors().Has("a") {
		t.Errorf("AddRule(required) must apply to the whole expression, got %v", vd.Errors().All())
	}
}

// completesWithin runs fn with a watchdog; a hang (the bug) trips the timeout.
func completesWithin(t *testing.T, name string, fn func()) {
	t.Helper()
	done := make(chan struct{}, 1)
	go func() {
		defer func() { _ = recover(); done <- struct{}{} }()
		fn()
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s: hung (unbounded deref loop not bounded)", name)
	}
}

// regex / not_regex share regexMatch: opposite verdicts, same fail-closed behavior.
func TestRegexMatchConsolidation(t *testing.T) {
	cases := []struct {
		rule, val string
		wantFail  bool
	}{
		{`regex:^[a-z]+$`, "abc", false},
		{`regex:^[a-z]+$`, "ABC", true},
		{`not_regex:^[a-z]+$`, "ABC", false},
		{`not_regex:^[a-z]+$`, "abc", true},
		{`regex:^[a-z]+$`, "", false}, // empty passes (omitempty)
		{`not_regex:^[a-z]+$`, "", false},
		{`regex:(`, "x", true}, // invalid pattern is rejected during construction
		{`not_regex:(`, "x", true},
	}
	for _, c := range cases {
		v, err := Value(c.val, c.rule)
		if err != nil {
			if c.rule == `regex:(` || c.rule == `not_regex:(` {
				continue
			}
			t.Fatal(err)
		}
		v.Validate(context.Background())
		if v.Fails() != c.wantFail {
			t.Errorf("%s on %q: fails=%v want %v (%v)", c.rule, c.val, v.Fails(), c.wantFail, v.Errors().All())
		}
	}
}

// date / datetime share parsesAsTime: layout arg, default layouts, real time.Time, blank rejected.
func TestParsesAsTimeConsolidation(t *testing.T) {
	cases := []struct {
		rule, val string
		wantFail  bool
	}{
		{"date", "2024-01-02", false},
		{"date", "not-a-date", true},
		{"date", "   ", true}, // blank rejected
		{"date", "", false},   // empty skipped (omitempty)
		{"datetime:2006-01-02", "2024-01-02", false},
		{"datetime:2006-01-02", "01/02/2024", true}, // wrong layout
		{"datetime", "2024-01-02T15:04:05Z", false}, // no-arg -> default layouts
	}
	for _, c := range cases {
		v := MustValue(c.val, c.rule)
		v.Validate(context.Background())
		if v.Fails() != c.wantFail {
			t.Errorf("%s on %q: fails=%v want %v (%v)", c.rule, c.val, v.Fails(), c.wantFail, v.Errors().All())
		}
	}
	// a real time.Time is accepted
	tv := MustValue(time.Now(), "date")
	tv.Validate(context.Background())
	if tv.Fails() {
		t.Errorf("a real time.Time should pass date, got %v", tv.Errors().All())
	}
}

// The omitempty predicate is exported as IsEmpty.
func TestIsEmptyExported(t *testing.T) {
	if !IsEmpty("") || !IsEmpty(nil) || !IsEmpty(0) || !IsEmpty([]string(nil)) {
		t.Error("IsEmpty should report empties as empty")
	}
	if IsEmpty("x") || IsEmpty(1) || IsEmpty([]int{1}) {
		t.Error("IsEmpty should report non-empties as non-empty")
	}
}

// RemoveRules/RemoveFilters are DSL-aware: a separator inside a quoted/raw/escaped arg isn't structural.
func TestRemoveRulesDSLAware(t *testing.T) {
	vd := MustMap(map[string]any{"x": "abc"}, map[string]string{})
	if err := vd.AddRules("x", `regex:"a && b"`); err != nil {
		t.Fatal(err)
	}
	if err := vd.AddRules("x", "required"); err != nil {
		t.Fatal(err)
	}
	// removing required must leave the regex (quoted arg has a literal " && ") intact, not split it.
	if err := vd.RemoveRules("x", "required"); err != nil {
		t.Fatal(err)
	}
	if got := vd.Rules()["x"]; got != `regex:"a && b"` {
		t.Errorf("RemoveRules corrupted the quoted-arg rule: got %q", got)
	}
	// removing the last rule drops the field
	if err := vd.RemoveRules("x", "regex"); err != nil {
		t.Fatal(err)
	}
	if _, ok := vd.Rules()["x"]; ok {
		t.Errorf("removing the last rule should drop the field, got %v", vd.Rules())
	}
}

// a nil *struct must validate as its zero value (applying field tags), not degrade to a single value.
func TestNilStructPointerAppliesTags(t *testing.T) {
	type User struct {
		Email string `validate:"notblank"`
	}
	var u *User // nil pointer
	nilVD := MustStruct(u)
	nilVD.Validate(context.Background())
	zeroVD := MustStruct(User{})
	zeroVD.Validate(context.Background())
	if !nilVD.Fails() {
		t.Error("a nil *struct must apply its tags (notblank fails on the zero Email), not bypass validation")
	}
	if nilVD.Fails() != zeroVD.Fails() {
		t.Errorf("nil *User and User{} must validate identically: nil=%v zero=%v", nilVD.Fails(), zeroVD.Fails())
	}

	// strict required is enforced through a nil pointer too
	type Acct struct {
		Name string `validate:"required"`
	}
	var a *Acct
	sd := MustNew(WithStrictRequired()).MustStruct(a)
	sd.Validate(context.Background())
	if !sd.Fails() {
		t.Error("a nil *struct under strict required must fail (zero Name), not bypass")
	}
}

func (nilSliceFilter) Signature() string { return "r5nilslice" }

func (nilSliceFilter) Handle(any, ...string) (any, error) { var s []string; return s, nil }

type rawArgFallibleRule struct{}

func (rawArgFallibleRule) Signature() string { return "r6raw" }

func (rawArgFallibleRule) IsRawArg() bool { return true }

func (rawArgFallibleRule) Validate(f *Field) (bool, error) {
	if a := f.Attrs(); len(a) == 1 && a[0] == "a,b" {
		return true, nil
	}
	return false, nil
}

func TestFallibleRuleRawArg(t *testing.T) {
	v := MustNew(WithFallibleRules(rawArgFallibleRule{}))
	vd := v.MustValue("x", `r6raw:a,b`)
	vd.Validate(context.Background())
	if vd.Fails() {
		t.Errorf("a fallible rule with IsRawArg must receive the raw unsplit arg, got %v", vd.Errors().All())
	}
}

// an all-failing OR chain surfaces one "||" parent in the public set; any satisfied branch passes.
func TestOrChainAggregation(t *testing.T) {
	allFail := MustValue("ZZZ", "in:a || in:b || in:c || in:d")
	allFail.Validate(context.Background())
	if !allFail.Fails() {
		t.Error("an all-failing OR chain must fail")
	}
	// the public set must carry the "||" parent (the four in: branches dedup to one "in").
	if got := allFail.Errors().Messages("value"); got["||"] == "" {
		t.Errorf("an all-failing OR must surface the || parent, got %v", got)
	}
	oneOK := MustValue("c", "in:a || in:b || in:c || in:d")
	oneOK.Validate(context.Background())
	if oneOK.Fails() {
		t.Errorf("an OR chain with a satisfied branch must pass, got %v", oneOK.Errors().All())
	}
}

// OR diagnostic aggregation must be linear, not quadratic, on a large all-failing chain.
func TestOrChainNoQuadraticBlowup(t *testing.T) {
	var b strings.Builder
	for i := range 20000 {
		if i > 0 {
			b.WriteString(" || ")
		}
		b.WriteString("in:x")
	}
	expr := b.String()
	completesWithin(t, "large all-failing OR chain", func() {
		v := MustValue("zzz", expr)
		v.Validate(context.Background())
		if !v.Fails() {
			t.Error("the all-failing OR chain should fail")
		}
	})
}

// AddRule must parenthesize an added top-level || rule, so the existing rule isn't grafted onto one branch.
func TestAddRuleOrRuleParenthesized(t *testing.T) {
	v := MustNew()
	vd := v.MustMap(map[string]any{"a": "123"}, map[string]string{"a": "boolean"})
	_ = vd.AddRules("a", "alpha || numeric")
	vd.Validate(context.Background())
	// "boolean && (alpha || numeric)" fails "123"; without the paren it would pass via numeric.
	if !vd.Errors().Has("a") {
		t.Errorf("an added || rule must be parenthesized when ANDed; got no error: %v", vd.Errors().All())
	}
}

// Errors is a pure collection (no error embed); Err() is nil on success.
func TestErrorsPureCollectionAndErr(t *testing.T) {
	ok := MustMap(map[string]any{"a": "x"}, map[string]string{"a": "alpha"})
	ok.Validate(context.Background())
	if ok.Err() != nil {
		t.Errorf("Err() must be nil on success, got %v", ok.Err())
	}
	if ok.Errors() == nil {
		t.Error("Errors() must never be nil")
	}
	if ok.Errors().String() != "" {
		t.Errorf("String() must be empty on success, got %q", ok.Errors().String())
	}

	bad := MustMap(map[string]any{"a": "9"}, map[string]string{"a": "alpha"})
	bad.Validate(context.Background())
	if err := bad.Err(); err == nil {
		t.Error("Err() must be non-nil on failure")
	} else if err.Error() == "" {
		t.Error("Err().Error() should carry the aggregated message")
	}
	if bad.Errors().String() == "" {
		t.Error("String() should carry the aggregated message on failure")
	}
}

// AddRule/AddFilters validate eagerly (fail fast) and leave the field intact.
func TestAddRuleEagerValidate(t *testing.T) {
	v := MustNew()
	vd := v.MustMap(map[string]any{}, map[string]string{})
	if err := vd.AddRules("a", "required && nosuchrule"); err == nil {
		t.Error("AddRule with an unknown rule should error")
	}
	if err := vd.AddRules("a", "required && ("); err == nil {
		t.Error("AddRule with a syntax error should error")
	}
	if vd.Rules()["a"] != "" {
		t.Errorf("a failed AddRule must leave the field unchanged, got %q", vd.Rules()["a"])
	}
	if err := vd.AddRules("a", "required && email"); err != nil {
		t.Errorf("a valid AddRule should not error: %v", err)
	}
	if err := vd.AddRules("a", "dive && email"); err != nil {
		t.Errorf("a valid diving AddRule should not error: %v", err)
	}
	if err := vd.AddFilters("a", "nosuchfilter"); err == nil {
		t.Error("AddFilters with an unknown filter should error")
	}
	if err := vd.AddFilters("a", "trim|lower"); err != nil {
		t.Errorf("valid filters should not error: %v", err)
	}
}

// a literal field name ending in ']' must not pick up base-name config.
func TestLiteralBracketFieldNoLeak(t *testing.T) {
	v := MustNew(
		WithMessages(map[string]string{"items.alpha": "LEAKED"}),
		WithAttributes(map[string]string{"items": "Items"}),
	)
	vd := v.MustMap(map[string]any{"items[0]": "9"}, map[string]string{"items[0]": "alpha"})
	vd.Validate(context.Background())
	got := vd.Errors().OneFor("items[0]")
	if got == "LEAKED" || strings.Contains(got, "Items") {
		t.Errorf("a literal field items[0] must NOT inherit items.* config, got %q", got)
	}
}

type failingFallibleRule struct{ sig string }

func (r failingFallibleRule) Signature() string { return r.sig }

func (r failingFallibleRule) Validate(*Field) (bool, error) { return false, nil }

// validate:"-" skips the whole subtree's validation, yet the subtree stays bindable.
func TestDashStructSubtreeNotValidated(t *testing.T) {
	type Inner struct {
		Email string `validate:"email"`
	}
	type Outer struct {
		Name   string `validate:"required"`
		Nested Inner  `validate:"-"`
	}
	vd := MustStruct(Outer{Name: "ok", Nested: Inner{Email: "not-an-email"}})
	vd.Validate(context.Background())
	if vd.Fails() {
		t.Errorf(`validate:"-" struct must skip its whole subtree's validation, got %v`, vd.Errors().All())
	}
	// subtree still binds
	var out Outer
	if err := vd.Bind(&out); err != nil {
		t.Fatal(err)
	}
	if out.Nested.Email != "not-an-email" {
		t.Errorf(`a "-" struct subtree must still bind, got %q`, out.Nested.Email)
	}

	// embedded form
	type Embed struct {
		Score int `validate:"gte:100"`
	}
	type T struct {
		Embed `validate:"-"`
		Name  string `validate:"required"`
	}
	vd2 := MustStruct(T{Embed: Embed{Score: 5}, Name: "n"})
	vd2.Validate(context.Background())
	if vd2.Fails() {
		t.Errorf(`validate:"-" embedded must skip its subtree's validation, got %v`, vd2.Errors().All())
	}
}

// a whitespace-only rule expression is a no-op (like "").
func TestWhitespaceExprNoOp(t *testing.T) {
	vd := MustMap(map[string]any{"a": "x"}, map[string]string{"a": "   "})
	vd.Validate(context.Background())
	if vd.Fails() {
		t.Errorf("a whitespace-only rule expression should be a no-op, got %v", vd.Errors().All())
	}
	type S struct {
		X string `validate:" "`
	}
	sv := MustStruct(S{X: "y"})
	sv.Validate(context.Background())
	if sv.Fails() {
		t.Errorf("a whitespace validate tag should be a no-op, got %v", sv.Errors().All())
	}
}

func (r *nilStringer) String() string { return r.s }

// AddRules with a whitespace-only rule is a no-op, not a parse error.
func TestAddRulesWhitespaceNoOp(t *testing.T) {
	vd := MustMap(map[string]any{"x": "ok"}, map[string]string{})
	if err := vd.AddRules("x", "   "); err != nil {
		t.Errorf("AddRules with a whitespace-only rule should be a no-op, got error: %v", err)
	}
	vd.Validate(context.Background())
	if vd.Fails() {
		t.Errorf("a whitespace AddRules should not produce a validation error, got %v", vd.Errors().All())
	}
}

// AddRules onto a whitespace-only existing expression treats it as absent (not "   && alpha").
func TestAddRulesOntoWhitespaceExisting(t *testing.T) {
	vd := MustMap(map[string]any{"x": "9"}, map[string]string{"x": "   "})
	if err := vd.AddRules("x", "alpha"); err != nil {
		t.Fatalf("AddRules onto a whitespace-existing field should not error: %v", err)
	}
	vd.Validate(context.Background())
	if !vd.Errors().Has("x") {
		t.Errorf("the added alpha rule should apply (9 fails alpha); got %v", vd.Errors().All())
	}
}

// conv.ToInt truncates fractional floats toward zero; overflow/non-finite still error.
func TestToIntTruncatesFractional(t *testing.T) {
	cases := []struct {
		in   any
		want int64
	}{
		{2.5, 2}, {-3.7, -3}, {"3.99", 3}, {float32(1.9), 1}, {12.0, 12},
	}
	for _, c := range cases {
		got, err := conv.ToInt(c.in)
		if err != nil || got != c.want {
			t.Errorf("ToInt(%v) = %d, %v; want %d, nil", c.in, got, err, c.want)
		}
	}
	if _, err := conv.ToInt(math.Inf(1)); err == nil {
		t.Error("ToInt(+Inf) must still error")
	}
	if _, err := conv.ToInt("9999999999999999999999"); err == nil {
		t.Error("ToInt(overflow string) must still error")
	}
}

// TestValid covers the pooled fast path and asserts it agrees with the full Struct path.
func TestValid(t *testing.T) {
	v := MustNew()
	cases := []struct {
		name string
		data benchUser
		want bool
	}{
		{"all valid", benchUser{Name: "alice", Email: "a@b.com", Age: 30}, true},
		{"bad alpha", benchUser{Name: "al1ce", Email: "a@b.com", Age: 30}, false},
		{"bad email", benchUser{Name: "alice", Email: "nope", Age: 30}, false},
		{"age too low", benchUser{Name: "alice", Email: "a@b.com", Age: 5}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := validResult(t, v, c.data)
			if got != c.want {
				t.Errorf("Valid() = %v, want %v", got, c.want)
			}
			vd := v.MustStruct(c.data)
			vd.Validate(context.Background())
			if (!vd.Fails()) != got {
				t.Errorf("Valid()=%v disagrees with MustStruct().Fails()=%v", got, vd.Fails())
			}
		})
	}
}

// TestValidZeroAlloc guards the flagship guarantee: Valid() allocates nothing
// on either verdict for pure-engine rules.
func TestValidZeroAlloc(t *testing.T) {
	type u struct {
		Name string `validate:"required && alpha && min:3"`
		Age  int    `validate:"required && gte:18 && lte:120"`
	}
	v := MustNew()
	// pre-boxed
	var pass any = u{Name: "alice", Age: 30}
	var fail any = u{Name: "al1ce", Age: 5}
	if a := testing.AllocsPerRun(200, func() { _, _ = v.Valid(context.Background(), pass) }); a != 0 {
		t.Errorf("Valid(success) allocated %v times, want 0", a)
	}
	if a := testing.AllocsPerRun(200, func() { _, _ = v.Valid(context.Background(), fail) }); a != 0 {
		t.Errorf("Valid(failure) allocated %v times, want 0", a)
	}
}

// The rules-map plan cache verifies caller-map content on every pointer hit.
func TestMapPlanCacheFreshness(t *testing.T) {
	v := MustNew()
	rules := map[string]string{"a": "alpha"}
	one := v.MustMap(map[string]any{"a": "abc"}, rules)
	one.Validate(context.Background())
	if one.Fails() {
		t.Fatalf("alpha should pass, got %v", one.Errors().All())
	}

	// mutate the same map object: the next validation must follow the new content
	rules["a"] = "numeric"
	two := v.MustMap(map[string]any{"a": "abc"}, rules)
	two.Validate(context.Background())
	if !two.Fails() {
		t.Error("a mutated caller rules map must not be served the stale plan")
	}

	// Validators have independent immutable rule registries.
	rv := MustNew()
	r1 := rv.MustMap(map[string]any{"x": "abc"}, map[string]string{"x": "alpha"})
	r1.Validate(context.Background())
	if r1.Fails() {
		t.Fatal("builtin alpha should pass")
	}
	overridden := MustNew(WithoutBuiltinRules(),
		WithRuleFunc("alpha", func(*Field) bool { return false }, "m"))
	r2 := overridden.MustMap(map[string]any{"x": "abc"}, map[string]string{"x": "alpha"})
	r2.Validate(context.Background())
	if !r2.Fails() {
		t.Error("custom immutable registry did not use its alpha rule")
	}

	// AddRules on one validation must not leak into another sharing the plan
	shared := map[string]string{"y": "alpha"}
	va := v.MustMap(map[string]any{"y": "abc"}, shared)
	if err := va.AddRules("y", "min:10"); err != nil {
		t.Fatal(err)
	}
	vb := v.MustMap(map[string]any{"y": "abc"}, shared)
	vb.Validate(context.Background())
	if vb.Fails() {
		t.Errorf("AddRules on a sibling validation must not leak into the shared plan, got %v", vb.Errors().All())
	}
	va.Validate(context.Background())
	if !va.Fails() {
		t.Error("the AddRules validation itself must enforce the added rule")
	}
}

// TestValidDive exercises the fast path's dive handling over slice and map fields.
func TestValidDive(t *testing.T) {
	type bag struct {
		Tags []string          `validate:"dive && alpha"`
		Meta map[string]string `validate:"dive && numeric"`
	}
	v := MustNew()
	if !validResult(t, v, bag{Tags: []string{"foo", "bar"}, Meta: map[string]string{"a": "1"}}) {
		t.Error("valid dive elements should pass")
	}
	if validResult(t, v, bag{Tags: []string{"foo", "b4r"}}) {
		t.Error("a non-alpha slice element should fail dive")
	}
	if validResult(t, v, bag{Meta: map[string]string{"a": "x"}}) {
		t.Error("a non-numeric map element should fail dive")
	}
}

func TestValidationParallel(t *testing.T) {
	type wide struct {
		A string `validate:"required && alpha"`
		B string `validate:"required && alpha"`
		C string `validate:"required && alpha"`
		D string `validate:"required && alpha"`
	}
	v := MustNew(WithParallel(2))
	valid := v.MustStruct(wide{A: "a", B: "b", C: "c", D: "d"})
	if err := valid.Validate(t.Context()); err != nil {
		t.Error("all-valid wide struct should pass under parallel")
	}
	invalid := v.MustStruct(wide{A: "a", B: "2", C: "c", D: "d"})
	if err := invalid.Validate(t.Context()); err == nil {
		t.Error("a non-alpha field should fail under parallel")
	}
}

// fieldAccessRule exercises the Field accessors: Context, Root, Name, Sibling.
type fieldAccessRule struct{}

func (fieldAccessRule) Signature() string { return "fieldaccess" }
func (fieldAccessRule) Message() string   { return "{field} failed fieldaccess" }
func (fieldAccessRule) Passes(f *Field) bool {
	if _, ok := f.Root[any](); f.Context() == nil || !ok || f.Name() == "" {
		return false
	}
	other, ok := f.Sibling[string]("Other")
	return ok && other == "ok"
}

// dataOnlyRule reads just Root(), reaching the map/value source.raw() paths.
type dataOnlyRule struct{}

func (dataOnlyRule) Signature() string { return "dataonly" }
func (dataOnlyRule) Message() string   { return "{field} failed dataonly" }
func (dataOnlyRule) Passes(f *Field) bool {
	_, ok := f.Root[any]()
	return ok
}

// TestCustomRulesAndFieldAccessors covers custom rules and Field accessors.
func TestCustomRulesAndFieldAccessors(t *testing.T) {
	v := MustNew(WithRules(fieldAccessRule{}, dataOnlyRule{}))
	ctx := context.Background()

	check := func(name string, vd *Validation, wantPass bool) {
		t.Helper()
		vd.Validate(ctx)
		if vd.Fails() == wantPass {
			t.Errorf("%s: Fails()=%v, want pass=%v (%v)", name, vd.Fails(), wantPass, vd.Errors().All())
		}
	}

	// struct source
	type payload struct {
		Target string `validate:"fieldaccess"`
		Other  string
	}
	check("struct pass", v.MustStruct(payload{Target: "x", Other: "ok"}), true)
	check("struct fail", v.MustStruct(payload{Target: "x", Other: "no"}), false)

	// map source
	check("map pass", v.MustMap(map[string]any{"Target": "x", "Other": "ok"}, map[string]string{"Target": "fieldaccess"}), true)
	check("map fail", v.MustMap(map[string]any{"Target": "x", "Other": "no"}, map[string]string{"Target": "fieldaccess"}), false)

	// var source
	check("var data", v.MustValue("anything", "dataonly"), true)
}

// TestFieldNilDefensiveAccessors covers the defensive nil branches of Root/Context.
func TestFieldNilDefensiveAccessors(t *testing.T) {
	f := &Field{name: "x"} // no vd, no ctx
	if _, ok := f.Root[any](); ok {
		t.Error("Root() with a nil validation should report false")
	}
	if f.Context() == nil {
		t.Error("Context() with a nil ctx should fall back to non-nil Background")
	}
}

// ClearRules/ClearFilters drop a field's whole expression/chain; Remove* with no names is a no-op.
func TestClearRulesAndFilters(t *testing.T) {
	vd := MustMap(map[string]any{"x": "abc"}, map[string]string{"x": "required && alpha"})
	if err := vd.RemoveRules("x"); err != nil { // no names -> no-op
		t.Fatal(err)
	}
	if vd.Rules()["x"] != "required && alpha" {
		t.Errorf("RemoveRules with no names must be a no-op, got %q", vd.Rules()["x"])
	}
	if err := vd.ClearRules("x"); err != nil {
		t.Fatal(err)
	}
	if _, ok := vd.Rules()["x"]; ok {
		t.Error("ClearRules must drop the field's expression")
	}

	fv := MustMap(map[string]any{"y": "  hi  "}, map[string]string{"y": "required"})
	if err := fv.AddFilters("y", "trim"); err != nil {
		t.Fatal(err)
	}
	if err := fv.RemoveFilters("y"); err != nil { // no names -> no-op
		t.Fatal(err)
	}
	if fv.Filters()["y"] != "trim" {
		t.Errorf("RemoveFilters with no names must be a no-op, got %q", fv.Filters()["y"])
	}
	if err := fv.ClearFilters("y"); err != nil {
		t.Fatal(err)
	}
	if _, ok := fv.Filters()["y"]; ok {
		t.Error("ClearFilters must drop the field's chain")
	}
}

// sometimes: PATCH semantics — an absent key skips every rule, a present key validates fully.
func TestSometimes(t *testing.T) {
	rules := map[string]string{"email": "sometimes && required && email"}
	v := MustNew()

	absent := v.MustMap(map[string]any{}, rules)
	absent.Validate(context.Background())
	if absent.Fails() {
		t.Errorf("absent key must skip sometimes-guarded rules: %v", absent.Errors().All())
	}
	bad := v.MustMap(map[string]any{"email": "nope"}, rules)
	bad.Validate(context.Background())
	if !bad.Fails() {
		t.Error("present key must run the full chain")
	}
	good := v.MustMap(map[string]any{"email": "a@b.com"}, rules)
	good.Validate(context.Background())
	if good.Fails() {
		t.Errorf("valid present value must pass: %v", good.Errors().All())
	}

	// struct: a nil pointer field counts as absent
	type Patch struct {
		Name *string `validate:"sometimes && required && min:3"`
	}
	np := v.MustStruct(&Patch{})
	np.Validate(context.Background())
	if np.Fails() {
		t.Errorf("nil pointer + sometimes must skip: %v", np.Errors().All())
	}
	short := "ab"
	sp := v.MustStruct(&Patch{Name: &short})
	sp.Validate(context.Background())
	if !sp.Fails() {
		t.Error("present pointer must validate min:3")
	}
	// Valid() fast path agrees
	if !validResult(t, v, &Patch{}) {
		t.Error("Valid: nil pointer + sometimes must pass")
	}
	if validResult(t, v, &Patch{Name: &short}) {
		t.Error("Valid: short present value must fail")
	}
}

// An explicit null is a PRESENT key: it must flow into required and fail, not
// be skipped as absent (else {"email": null} bypasses sometimes && required).
func TestSometimesExplicitNull(t *testing.T) {
	rules := map[string]string{"email": "sometimes && required && email"}

	null := MustJSON(`{"email": null}`, rules)
	null.Validate(context.Background())
	if !null.Fails() {
		t.Error("explicit JSON null must fail required, not skip as absent")
	}
	mapNil := MustMap(map[string]any{"email": nil}, rules)
	mapNil.Validate(context.Background())
	if !mapNil.Fails() {
		t.Error("a present nil map value must fail required")
	}
	missing := MustJSON(`{}`, rules)
	missing.Validate(context.Background())
	if missing.Fails() {
		t.Errorf("a truly missing key must still skip: %v", missing.Errors().All())
	}
}

// sometimes with dive: an absent collection skips element rules too.
func TestSometimesDive(t *testing.T) {
	rules := map[string]string{"tags": "sometimes && required && dive && min:2"}
	absent := MustMap(map[string]any{}, rules)
	absent.Validate(context.Background())
	if absent.Fails() {
		t.Errorf("absent collection must skip dive rules: %v", absent.Errors().All())
	}
	bad := MustMap(map[string]any{"tags": []any{"ok", "x"}}, rules)
	bad.Validate(context.Background())
	if !bad.Fails() {
		t.Error("present collection must dive-validate elements")
	}
}

// sometimes is an always-true marker: under ||/! or in a dive element it is a
// vacuous-pass / silent no-op, so those placements are compile-time errors.
func TestSometimesPlacementRejected(t *testing.T) {
	for name, rule := range map[string]string{
		"under or":     "sometimes || notblank",
		"under not":    "!sometimes",
		"dive element": "dive && sometimes && notblank",
	} {
		if _, err := Map(map[string]any{"x": ""}, map[string]string{"x": rule}); !errors.Is(err, ErrInvalidRules) {
			t.Errorf("%s (%q) error = %v", name, rule, err)
		}
	}
	// AddRules fail-fast agrees
	av := MustMap(map[string]any{}, map[string]string{})
	if err := av.AddRules("x", "sometimes || required"); err == nil {
		t.Error("AddRules must reject sometimes under ||")
	}
	if err := av.AddRules("xs", "dive && sometimes"); err == nil {
		t.Error("AddRules must reject sometimes in a dive element")
	}
	// the legal spine placement still compiles
	if err := av.AddRules("ok", "sometimes && required"); err != nil {
		t.Errorf("legal sometimes placement must compile: %v", err)
	}
}

func TestCheck(t *testing.T) {
	type Good struct {
		Email string `validate:"required && email"`
		Inner struct {
			Port int `validate:"port"`
		}
	}
	if err := Check[Good](); err != nil {
		t.Errorf("Check on valid tags: %v", err)
	}
	type Bad struct {
		A string `validate:"required && no_such_rule"`
		B string `validate:"regex:\"[\""`
	}
	err := Check[Bad]()
	if err == nil {
		t.Fatal("Check must report bad tags")
	}
	msg := err.Error()
	if !strings.Contains(msg, `field "A"`) || !strings.Contains(msg, `field "B"`) {
		t.Errorf("both bad fields must be reported, got %q", msg)
	}
	if err := Check[int](); err == nil {
		t.Error("non-struct input must error")
	}
}

func TestStringRuleOption(t *testing.T) {
	v := MustNew(WithStringRule("prefixed", func(s string, args ...string) bool {
		return len(args) > 0 && strings.HasPrefix(s, args[0])
	}, "{field} must start with {0}"))

	bad := v.MustMap(map[string]any{"x": "world"}, map[string]string{"x": "prefixed:hello"})
	bad.Validate(context.Background())
	if !bad.Fails() {
		t.Error("prefixed:hello must fail on world")
	}
	good := v.MustMap(map[string]any{"x": "helloworld"}, map[string]string{"x": "prefixed:hello"})
	good.Validate(context.Background())
	if good.Fails() {
		t.Errorf("prefixed:hello must pass on helloworld: %v", good.Errors().All())
	}
	// omitempty is pre-applied: empty value never reaches fn
	empty := v.MustMap(map[string]any{"x": ""}, map[string]string{"x": "prefixed:hello"})
	empty.Validate(context.Background())
	if empty.Fails() {
		t.Error("WithStringRule must skip empty values")
	}
	// non-string values arrive rendered as strings
	num := v.MustMap(map[string]any{"x": 12345}, map[string]string{"x": "prefixed:123"})
	num.Validate(context.Background())
	if num.Fails() {
		t.Errorf("numeric value must render as string for fn: %v", num.Errors().All())
	}
}

// DescribeRules must report rules on struct-valued (non-leaf) fields:
// `Profile *Profile validate:"required"` is enforced at runtime, so hiding it
// from introspection would let an OpenAPI consumer publish a false contract.
func TestDescribeRulesStructField(t *testing.T) {
	type profile struct {
		Bio string `json:"bio" validate:"max:100"`
	}
	type user struct {
		Name    string   `json:"name" validate:"required"`
		Profile *profile `json:"profile" validate:"required"`
	}
	frs, err := Describe[user]()
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string][]RuleInfo{}
	for _, fr := range frs {
		byName[fr.Name] = fr.Rules
	}
	if len(byName["Profile"]) != 1 || byName["Profile"][0].Name != "required" {
		t.Errorf("struct-valued field rules must be reported, got %v", byName)
	}
	if len(byName["Profile.Bio"]) != 1 || byName["Profile.Bio"][0].Name != "max" {
		t.Errorf("nested leaf rules must still be reported, got %v", byName)
	}
}

func TestErrorsItemsAndAsErrors(t *testing.T) {
	vd := MustMap(map[string]any{"age": "3"}, map[string]string{"age": "min:5", "name": "required"})
	vd.Validate(context.Background())
	items := vd.Errors().Items()
	if len(items) != 2 {
		t.Fatalf("want 2 items, got %d: %v", len(items), items)
	}
	for _, it := range items {
		if it.Field == "" || it.Rule == "" || it.Message == "" {
			t.Errorf("item must be fully populated: %+v", it)
		}
		if strings.Contains(it.Message, "{field}") || strings.Contains(it.Message, "{0}") {
			t.Errorf("Items must resolve placeholders, got %q", it.Message)
		}
	}
	// mutating a returned item's params must not corrupt the compiled cache
	for i := range items {
		for j := range items[i].Params {
			items[i].Params[j] = "MUTATED"
		}
	}
	again := vd.Errors().Items()
	for _, it := range again {
		for _, p := range it.Params {
			if p == "MUTATED" {
				t.Fatal("Items must clone Params")
			}
		}
	}

	// AsErrors extracts the collection from Err(), including wrapped
	err := vd.Err()
	if err == nil {
		t.Fatal("Err must be non-nil on failure")
	}
	wrapped := fmt.Errorf("bind failed: %w", err)
	es, ok := AsErrors(wrapped)
	if !ok || !es.Has("age") {
		t.Error("AsErrors must extract through wrapping")
	}
	if _, ok := AsErrors(errors.New("plain")); ok {
		t.Error("AsErrors on a foreign error must be false")
	}
}
