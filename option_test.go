package validator

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestWithStrictRequired(t *testing.T) {
	ctx := context.Background()

	// default: present-but-empty satisfies required
	def := NewValidator().Map(map[string]any{"a": ""}, map[string]string{"a": "required"})
	def.Validate(ctx)
	if def.Fails() {
		t.Errorf("default required: present empty value should pass, got %v", def.Errors().All())
	}

	// strict: zero value fails required
	strict := NewValidator(WithStrictRequired()).Map(map[string]any{"a": ""}, map[string]string{"a": "required"})
	strict.Validate(ctx)
	if !strict.Fails() {
		t.Error("strict required: empty value should fail")
	}

	ok := NewValidator(WithStrictRequired()).Map(map[string]any{"a": "x"}, map[string]string{"a": "required"})
	ok.Validate(ctx)
	if ok.Fails() {
		t.Errorf("strict required: non-zero value should pass, got %v", ok.Errors().All())
	}
}

// WithStrictRequired applies to the whole required family, not just bare required.
func TestStrictRequiredFamily(t *testing.T) {
	ctx := context.Background()
	strict := NewValidator(WithStrictRequired())
	def := NewValidator()

	cases := []struct {
		name string
		data map[string]any
		sig  string
	}{
		{"required_if", map[string]any{"role": "admin", "level": 0}, "required_if:role,admin"},
		{"required_unless", map[string]any{"role": "user", "level": 0}, "required_unless:role,admin"},
		{"required_with", map[string]any{"a": "set", "level": 0}, "required_with:a"},
		{"required_without", map[string]any{"level": 0}, "required_without:a"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sv := strict.Map(c.data, map[string]string{"level": c.sig})
			sv.Validate(ctx)
			if !sv.Fails() {
				t.Errorf("strict %s: zero value should fail when required", c.sig)
			}
			dv := def.Map(c.data, map[string]string{"level": c.sig})
			dv.Validate(ctx)
			if dv.Fails() {
				t.Errorf("default %s: present zero value should pass, got %v", c.sig, dv.Errors().All())
			}
		})
	}
}

type withSecret struct {
	secret string `validate:"required && min:5"`
}

func TestWithPrivateFieldValidation(t *testing.T) {
	ctx := context.Background()

	// default: unexported fields are skipped
	def := NewValidator().Struct(withSecret{secret: "abc"})
	def.Validate(ctx)
	if def.Fails() {
		t.Errorf("default: unexported field should be skipped, got %v", def.Errors().All())
	}

	// enabled: the unexported field is validated
	priv := NewValidator(WithPrivateFieldValidation()).Struct(withSecret{secret: "abc"})
	priv.Validate(ctx)
	if !priv.Fails() {
		t.Error("private-field validation should validate the unexported field")
	}
}

func TestWithTransformFunc(t *testing.T) {
	v := NewValidator(WithTransformFunc(func(m string) string { return "[" + m + "]" }))
	vd := v.Map(map[string]any{"x": "nope"}, map[string]string{"x": "email"})
	vd.Validate(context.Background())
	got := vd.Errors().OneFor("x")
	if !strings.HasPrefix(got, "[") || !strings.HasSuffix(got, "]") {
		t.Errorf("TransformFunc should wrap the final message, got %q", got)
	}
}

// Catalog getters return copies so a caller cannot corrupt the shared catalog.
func TestCatalogGettersCopy(t *testing.T) {
	a := Rules()
	if len(a) == 0 {
		t.Fatal("expected built-in rules")
	}
	n := len(a)
	a[0] = nil
	if b := Rules(); len(b) != n || b[0] == nil {
		t.Error("Rules() must return a copy; mutating it corrupted the catalog")
	}
	if f := Filters(); len(f) > 0 {
		f[0] = nil
		if g := Filters(); g[0] == nil {
			t.Error("Filters() must return a copy")
		}
	}
}

// IsEmpty covers complex, so strict required rejects a zero complex value.
func TestStrictRequiredComplex(t *testing.T) {
	zero := NewValidator(WithStrictRequired()).Var(complex128(0), "required")
	zero.Validate(context.Background())
	if !zero.Fails() {
		t.Error("strict required on a zero complex must fail")
	}
	nz := NewValidator(WithStrictRequired()).Var(complex128(complex(1, 2)), "required")
	nz.Validate(context.Background())
	if nz.Fails() {
		t.Error("strict required on a non-zero complex should pass")
	}
}

// Strict required rejects a zero struct field.
func TestStrictRequiredZeroStruct(t *testing.T) {
	type Inner struct{ X int }
	type Outer struct {
		I Inner `validate:"required"`
	}
	sv := NewValidator(WithStrictRequired())
	zero := sv.Struct(Outer{})
	zero.Validate(context.Background())
	if !zero.Fails() {
		t.Error("strict required on a zero struct field must fail")
	}
	nz := sv.Struct(Outer{I: Inner{X: 1}})
	nz.Validate(context.Background())
	if nz.Fails() {
		t.Errorf("strict required on a non-zero struct field should pass, got %v", nz.Errors().All())
	}
}

// Strict required rejects a zero fixed-size array.
func TestStrictRequiredZeroArray(t *testing.T) {
	type T struct {
		A [3]int `validate:"required"`
	}
	sv := NewValidator(WithStrictRequired())
	zero := sv.Struct(T{})
	zero.Validate(context.Background())
	if !zero.Fails() {
		t.Error("strict required on a zero array must fail")
	}
	nz := sv.Struct(T{A: [3]int{0, 0, 1}})
	nz.Validate(context.Background())
	if nz.Fails() {
		t.Errorf("strict required on a non-zero array should pass, got %v", nz.Errors().All())
	}
}

// Map-valued options merge and copy the caller's map.
func TestOptionsMergeAndCopy(t *testing.T) {
	// copy: a later mutation of the caller's map must not leak in
	attrs := map[string]string{"Name": "Naam"}
	v := NewValidator(WithAttributes(attrs))
	attrs["Name"] = "HACKED"
	vd := v.Map(map[string]any{}, map[string]string{"Name": "required"})
	vd.Validate(context.Background())
	if msg := vd.Errors().OneFor("Name"); !strings.Contains(msg, "Naam") || strings.Contains(msg, "HACKED") {
		t.Errorf("WithAttributes must copy the caller map; msg=%q", msg)
	}

	// merge: repeated calls combine
	v2 := NewValidator(
		WithAttributes(map[string]string{"A": "Aye"}),
		WithAttributes(map[string]string{"B": "Bee"}),
	)
	vd2 := v2.Map(map[string]any{}, map[string]string{"A": "required", "B": "required"})
	vd2.Validate(context.Background())
	if !strings.Contains(vd2.Errors().OneFor("A"), "Aye") || !strings.Contains(vd2.Errors().OneFor("B"), "Bee") {
		t.Errorf("repeated WithAttributes must merge: A=%q B=%q", vd2.Errors().OneFor("A"), vd2.Errors().OneFor("B"))
	}
}

// A Register* override after an expression was compiled invalidates the cache.
func TestRegisterInvalidatesCache(t *testing.T) {
	v := NewValidator()
	first := v.Var("abc", "alpha")
	first.Validate(context.Background()) // compiles + caches builtin alpha
	v.RegisterFunc("alpha", func(Field) bool { return false }, "nope")
	vd := v.Var("abc", "alpha")
	vd.Validate(context.Background())
	if !vd.Fails() {
		t.Error("a RegisterFunc override after compile must take effect (stale cache)")
	}
}

// Under WithPrivateFieldValidation a by-value struct reads unexported composite fields.
func TestPrivateCompositeByValue(t *testing.T) {
	type Account struct {
		token  *string  `validate:"required"`
		labels []string `validate:"required"`
	}
	v := NewValidator(WithPrivateFieldValidation())
	tok := "abc"
	byVal := v.Struct(Account{token: &tok, labels: []string{"x"}})
	byVal.Validate(context.Background())
	if byVal.Fails() {
		t.Errorf("by-value unexported composite fields should read their values, got %v", byVal.Errors().All())
	}
}

// Rules are read from a custom struct tag.
func TestWithTagName(t *testing.T) {
	type user struct {
		Name string `valid:"required && alpha"`
	}
	ctx := context.Background()

	// custom tag "valid" is read: "1" is not alpha
	vd := NewValidator(WithTagName("valid")).Struct(user{Name: "1"})
	vd.Validate(ctx)
	if !vd.Fails() {
		t.Error(`WithTagName("valid") should read rules from the 'valid' tag`)
	}

	// default tag ignores 'valid', so no rules apply
	def := NewValidator().Struct(user{Name: "1"})
	def.Validate(ctx)
	if def.Fails() {
		t.Errorf("default tag should ignore the 'valid' tag, got %v", def.Errors().All())
	}
}

// The field/error name is derived per field, here from the json tag.
func TestWithTagNameFunc(t *testing.T) {
	type user struct {
		FullName string `validate:"required && alpha" json:"full_name"`
	}
	v := NewValidator(WithTagNameFunc(func(f reflect.StructField) string {
		return f.Tag.Get("json")
	}))
	vd := v.Struct(user{FullName: "1"})
	vd.Validate(context.Background())
	if !vd.Fails() {
		t.Fatal(`alpha should fail on "1"`)
	}
	if !vd.Errors().Has("full_name") {
		t.Errorf("field name should be derived to 'full_name' via WithTagNameFunc, got %v", vd.Errors().All())
	}
}
