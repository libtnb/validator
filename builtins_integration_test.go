package validator

import (
	"reflect"
	"testing"
)

// TestBuiltinCatalogNoDuplicateSignatures guards that no two builtins share a signature (silent registry overwrite).
func TestBuiltinCatalogNoDuplicateSignatures(t *testing.T) {
	seen := map[string]bool{}
	for _, r := range Rules() {
		if seen[r.Signature()] {
			t.Errorf("duplicate built-in rule signature: %q", r.Signature())
		}
		seen[r.Signature()] = true
	}
	for _, r := range ErrorRules() {
		if seen[r.Signature()] {
			t.Errorf("duplicate signature across rules/error-rules: %q", r.Signature())
		}
		seen[r.Signature()] = true
	}
	fseen := map[string]bool{}
	for _, f := range Filters() {
		if fseen[f.Signature()] {
			t.Errorf("duplicate built-in filter signature: %q", f.Signature())
		}
		fseen[f.Signature()] = true
	}

	// sanity: catalog should be substantial
	if n := len(Rules()); n < 40 {
		t.Errorf("expected >=40 built-in rules, got %d", n)
	}
	if n := len(Filters()); n < 8 {
		t.Errorf("expected >=8 built-in filters, got %d", n)
	}
}

// TestBuiltinRulesThroughDSL guards that registered builtins compile via the DSL and evaluate over a field.
func TestBuiltinRulesThroughDSL(t *testing.T) {
	v := NewValidator()

	cases := []struct {
		expr string
		val  any
		want bool
	}{
		{"required && email", "a@b.com", true},
		{"required && email", "not-an-email", false},
		{"required && email", "", true}, // default required: "" present; email skips empty
		{"required && (in:a,b || alpha)", "a", true},
		{"required && (in:a,b || alpha)", "xyz", true},
		{"required && (in:a,b || alpha)", "12", false},
		{`regex:"^x.+"`, "xylophone", true}, // raw-arg quoted regex
		{`regex:"^x.+"`, "nope", false},
		{"min:3", "abcd", true},
		{"min:3", "ab", false},
		{"min:3", 5, true}, // numeric value, not length
		{"max:10 && numeric", "7", true},
	}
	for _, c := range cases {
		compiled, err := v.compile(c.expr)
		if err != nil {
			t.Errorf("compile(%q): %v", c.expr, err)
			continue
		}
		f := &field{name: "field", rv: reflect.ValueOf(c.val)}
		if got := compiled.Fast(f); got != c.want {
			t.Errorf("%q over %v = %v, want %v", c.expr, c.val, got, c.want)
		}
	}
}

// TestBuiltinFilters spot-checks a couple of built-in filters end to end.
func TestBuiltinFilters(t *testing.T) {
	v := NewValidator()
	trim, ok := v.registry.filter("trim")
	if !ok {
		t.Fatal("trim filter not registered")
	}
	out, err := trim.Handle("  hi  ")
	if err != nil || out != "hi" {
		t.Errorf("trim(\"  hi  \") = %v, %v", out, err)
	}
	lower, ok := v.registry.filter("lower")
	if !ok {
		t.Fatal("lower filter not registered")
	}
	if out, _ := lower.Handle("ABC"); out != "abc" {
		t.Errorf("lower(ABC) = %v", out)
	}
}
