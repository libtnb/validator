package validator

import (
	"context"
	"strings"
	"testing"

	"github.com/libtnb/validator/translations"
)

func TestMessageParamSubstitution(t *testing.T) {
	v := NewValidator()
	vd := v.Map(map[string]any{"age": "3"}, map[string]string{"age": "min:5"})
	vd.Validate(context.Background())
	msg := vd.Errors().OneFor("age")
	// {field}->age, {0}->5
	if !strings.Contains(msg, "age") || !strings.Contains(msg, "5") {
		t.Errorf("expected param-substituted message, got %q", msg)
	}
	if strings.Contains(msg, "{0}") || strings.Contains(msg, "{field}") {
		t.Errorf("placeholders not substituted: %q", msg)
	}
}

func TestMessageAttributeAlias(t *testing.T) {
	v := NewValidator(WithAttributes(map[string]string{"email": "Email Address"}))
	vd := v.Map(map[string]any{"email": "nope"}, map[string]string{"email": "email"})
	vd.Validate(context.Background())
	if got := vd.Errors().OneFor("email"); !strings.Contains(got, "Email Address") {
		t.Errorf("attribute alias not applied: %q", got)
	}
}

func TestMessageOverridePriority(t *testing.T) {
	v := NewValidator(WithMessages(map[string]string{
		"email.required": "field-level message",
		"required":       "rule-level message",
	}))

	// field-level override wins for email
	em := v.Map(map[string]any{}, map[string]string{"email": "required"})
	em.Validate(context.Background())
	if got := em.Errors().OneFor("email"); got != "field-level message" {
		t.Errorf("field-level override: %q", got)
	}

	// rule-level override for a different field
	nm := v.Map(map[string]any{}, map[string]string{"name": "required"})
	nm.Validate(context.Background())
	if got := nm.Errors().OneFor("name"); got != "rule-level message" {
		t.Errorf("rule-level override: %q", got)
	}
}

func TestMessageI18n(t *testing.T) {
	v := NewValidator(WithTranslation(translations.ZhHans()))
	vd := v.Map(map[string]any{"email": "nope"}, map[string]string{"email": "email"})
	vd.Validate(context.Background())
	if got := vd.Errors().OneFor("email"); !strings.Contains(got, "必须是有效的邮箱地址") {
		t.Errorf("i18n template not applied: %q", got)
	}
}

// The synthetic "||" parent message is translatable, so composite failures are
// not mixed-language under a locale pack.
func TestMessageI18nCompositeOr(t *testing.T) {
	v := NewValidator(WithTranslation(translations.ZhHans()))
	vd := v.Map(map[string]any{"x": "!!"}, map[string]string{"x": "alpha || numeric"})
	vd.Validate(context.Background())
	msgs := vd.Errors().Messages("x")
	or, ok := msgs["||"]
	if !ok {
		t.Fatalf("expected a '||' parent error, got %v", msgs)
	}
	if !strings.Contains(or, "不满足任一条件") {
		t.Errorf("the '||' parent message must use the locale template, got %q", or)
	}
}

func TestMessageOverrideBeatsTranslation(t *testing.T) {
	v := NewValidator(
		WithTranslation(translations.ZhHans()),
		WithMessages(map[string]string{"required": "custom wins"}),
	)
	vd := v.Map(map[string]any{}, map[string]string{"x": "required"})
	vd.Validate(context.Background())
	if got := vd.Errors().OneFor("x"); got != "custom wins" {
		t.Errorf("WithMessages should beat WithTranslation: %q", got)
	}
}

// TestTranslatorFunc: the map wins over the func when both supply a template.
func TestTranslatorFunc(t *testing.T) {
	v := NewValidator(WithTranslator(func(rule string) (string, bool) {
		if rule == "required" {
			return "dynamic: {field} needed", true
		}
		return "", false
	}))
	vd := v.Map(map[string]any{}, map[string]string{"name": "required"})
	vd.Validate(context.Background())
	if got := vd.Errors().OneFor("name"); got != "dynamic: name needed" {
		t.Errorf("translator func should supply the template, got %q", got)
	}

	// false falls through to the built-in template
	fall := v.Map(map[string]any{"x": "nope"}, map[string]string{"x": "email"})
	fall.Validate(context.Background())
	if got := fall.Errors().OneFor("x"); !strings.Contains(got, "valid email") {
		t.Errorf("a false from the translator func should fall back to built-in, got %q", got)
	}

	// map wins over func for the same rule
	both := NewValidator(
		WithTranslation(map[string]string{"required": "from map"}),
		WithTranslator(func(rule string) (string, bool) { return "from func", true }),
	)
	bd := both.Map(map[string]any{}, map[string]string{"y": "required"})
	bd.Validate(context.Background())
	if got := bd.Errors().OneFor("y"); got != "from map" {
		t.Errorf("WithTranslation map should win over WithTranslator func, got %q", got)
	}
}

func TestMessageNoDoubleExpansion(t *testing.T) {
	// a substituted value looking like a placeholder must not be re-expanded
	got := substitutePlaceholders("The {0} and {1}.", "x", []string{"{1}", "Z"})
	if got != "The {1} and Z." {
		t.Errorf("double-expansion: got %q want %q", got, "The {1} and Z.")
	}
}

// {N+} joins params[N:], so multi-value rules name every candidate value.
func TestMessageJoinedParams(t *testing.T) {
	got := substitutePlaceholders("required when {0} is {1+}.", "x", []string{"role", "admin", "root"})
	if got != "required when role is admin, root." {
		t.Errorf("{1+} join: got %q", got)
	}

	// end to end: required_if with multiple candidates lists them all
	vd := Map(
		map[string]any{"role": "root"},
		map[string]string{"pin": "required_if:role,admin,root"},
	)
	vd.Validate(context.Background())
	msg := vd.Errors().OneFor("pin")
	if !strings.Contains(msg, "admin, root") {
		t.Errorf("required_if message must list every candidate value, got %q", msg)
	}
}

func (rawArgErrorRule) Message() string { return "r6raw failed" }

// TestTransformFuncPanicRecovered: a panicking WithTransformFunc must be recovered,
// else message resolution re-panics into a fatal goroutine crash under WithParallel.
func TestTransformFuncPanicRecovered(t *testing.T) {
	v := NewValidator(WithTransformFunc(func(string) string { panic("boom") }))
	vd := v.Var("", "notblank")
	vd.Validate(context.Background())
	if !vd.Fails() {
		t.Error("notblank on empty should fail")
	}
	_ = vd.Errors().All()

	pv := NewValidator(WithParallel(1), WithTransformFunc(func(string) string { panic("boom") }))
	pd := pv.Map(map[string]any{"a": "", "b": ""}, map[string]string{"a": "notblank", "b": "notblank"})
	pd.Validate(context.Background())
	if !pd.Fails() {
		t.Error("expected failures under parallel")
	}
}

// TestTranslationCompleteness: every built-in rule has a template in each locale,
// and no locale carries a stray (typo'd) key matching no rule.
func TestTranslationCompleteness(t *testing.T) {
	locales := map[string]map[string]string{
		"ZhHans": translations.ZhHans(),
		"ZhHant": translations.ZhHant(),
		"Ja":     translations.Ja(),
		"Ko":     translations.Ko(),
		"Es":     translations.Es(),
		"Ru":     translations.Ru(),
	}
	valid := map[string]bool{}
	for _, r := range Rules() {
		valid[r.Signature()] = true
	}
	for _, r := range ErrorRules() {
		valid[r.Signature()] = true
	}
	// synthetic composite messages the engine emits itself
	valid["||"] = true
	valid["!"] = true
	for name, loc := range locales {
		for sig := range valid {
			if _, ok := loc[sig]; !ok {
				t.Errorf("translations.%s() is missing a template for rule %q", name, sig)
			}
		}
		for key := range loc {
			if !valid[key] {
				t.Errorf("translations.%s() has a stray key %q matching no built-in rule", name, key)
			}
		}
	}
}

// TestEmptyMessageFallback: an empty rule message still yields a non-blank failure.
func TestEmptyMessageFallback(t *testing.T) {
	v := NewValidator(WithoutBuiltinRules())
	v.RegisterFunc("mr", func(Field) bool { return false }, "")
	vd := v.Var("x", "mr")
	vd.Validate(context.Background())
	if got := vd.Errors().OneFor("value"); got == "" {
		t.Error("an empty rule message must fall back to a generic, non-blank message")
	}
}

func (r failErrorRule) Message() string { return "forced fail" }

// TestErrorRuleOverridesRule: a registered ErrorRule overrides a same-signature built-in Rule.
func TestErrorRuleOverridesRule(t *testing.T) {
	v := NewValidator()
	v.RegisterErrorRule(failErrorRule{sig: "alpha"})
	vd := v.Var("abc", "alpha") // passes built-in alpha; the override fails
	vd.Validate(context.Background())
	if !vd.Fails() {
		t.Error("a RegisterErrorRule should override a same-signature built-in Rule")
	}
}

// TestAddMessagesPerValidation: AddMessages overrides for one run only, beats the
// Validator's WithMessages, and does not leak to a sibling validation.
func TestAddMessagesPerValidation(t *testing.T) {
	v := NewValidator(WithMessages(map[string]string{"required": "global"}))

	vd := v.Map(map[string]any{}, map[string]string{"name": "required"})
	if err := vd.AddMessages(map[string]string{"required": "per-run"}); err != nil {
		t.Fatal(err)
	}
	vd.Validate(context.Background())
	if got := vd.Errors().OneFor("name"); got != "per-run" {
		t.Errorf("per-validation override should beat WithMessages: %q", got)
	}

	// a sibling validation from the same Validator keeps the global message
	sib := v.Map(map[string]any{}, map[string]string{"name": "required"})
	sib.Validate(context.Background())
	if got := sib.Errors().OneFor("name"); got != "global" {
		t.Errorf("AddMessages must not leak to a sibling validation: %q", got)
	}
}

// TestAddMessagesFieldRulePriority: within the override, "field.rule" beats "rule".
func TestAddMessagesFieldRulePriority(t *testing.T) {
	v := NewValidator()
	vd := v.Map(map[string]any{}, map[string]string{"email": "required", "name": "required"})
	_ = vd.AddMessages(map[string]string{
		"email.required": "email specific",
		"required":       "generic",
	})
	vd.Validate(context.Background())
	if got := vd.Errors().OneFor("email"); got != "email specific" {
		t.Errorf("field.rule override should win: %q", got)
	}
	if got := vd.Errors().OneFor("name"); got != "generic" {
		t.Errorf("rule override should apply to other fields: %q", got)
	}
}

// TestAddMessagesMergeAndCopy: repeat calls merge; the caller's map is copied.
func TestAddMessagesMergeAndCopy(t *testing.T) {
	v := NewValidator()
	vd := v.Map(map[string]any{}, map[string]string{"a": "required", "b": "required"})
	src := map[string]string{"a.required": "A"}
	_ = vd.AddMessages(src)
	_ = vd.AddMessages(map[string]string{"b.required": "B"})
	src["a.required"] = "MUTATED" // mutating the input after the call must not matter
	vd.Validate(context.Background())
	if got := vd.Errors().OneFor("a"); got != "A" {
		t.Errorf("input map should be copied, got %q", got)
	}
	if got := vd.Errors().OneFor("b"); got != "B" {
		t.Errorf("repeat AddMessages should merge, got %q", got)
	}
}

// TestAddMessagesBeatsTranslation: AddMessages outranks WithTranslation; empty input is a no-op.
func TestAddMessagesBeatsTranslation(t *testing.T) {
	v := NewValidator(WithTranslation(translations.ZhHans()))
	vd := v.Map(map[string]any{}, map[string]string{"x": "required"})
	_ = vd.AddMessages(map[string]string{"required": "override wins"})
	vd.Validate(context.Background())
	if got := vd.Errors().OneFor("x"); got != "override wins" {
		t.Errorf("AddMessages should beat WithTranslation: %q", got)
	}

	nop := v.Map(map[string]any{}, map[string]string{"x": "required"})
	_ = nop.AddMessages(nil)
	_ = nop.AddMessages(map[string]string{})
	nop.Validate(context.Background())
	if got := nop.Errors().OneFor("x"); !strings.Contains(got, "必填") {
		t.Errorf("empty AddMessages should be a no-op (translation kept): %q", got)
	}
}
