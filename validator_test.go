package validator

import (
	"context"
	"reflect"
	"testing"
)

func TestErrorsModel(t *testing.T) {
	e := &validationErrors{}
	if e.Error() != "" || e.Has("a") {
		t.Fatal("empty errors should be empty")
	}
	e.add(FieldError{Field: "email", Rule: "required", Message: "The email is required."}, "email")
	e.add(FieldError{Field: "email", Rule: "email", Message: "The email must be valid."}, "email")
	e.add(FieldError{Field: "age", Rule: "min", Message: "The age is too small."}, "age")

	if !e.Has("email") {
		t.Error("Has(email) should be true")
	}
	if e.OneFor("email") != "The email is required." {
		t.Errorf("One(email)=%q", e.OneFor("email"))
	}
	if e.One() != "The email is required." {
		t.Errorf("One()=%q", e.One())
	}
	if len(e.Messages("email")) != 2 {
		t.Errorf("Messages(email) len=%d want 2", len(e.Messages("email")))
	}
	all := e.All()
	if len(all) != 2 || len(all["email"]) != 2 {
		t.Errorf("All() projection wrong: %v", all)
	}
	// fields sorted
	want := "age: The age is too small.; email: The email is required."
	if got := e.Error(); got != want {
		t.Errorf("Error()=%q want %q", got, want)
	}
}

func TestRegistryAndFuncRule(t *testing.T) {
	v := NewValidator(WithoutBuiltinRules())
	called := false
	v.RegisterFunc("always", func(f Field) bool { called = true; return true }, "nope")

	r, ok := v.registry.rule("always")
	if !ok {
		t.Fatal("rule 'always' not registered")
	}
	f := &field{name: "x", rv: reflect.ValueOf("y")}
	if !r.Passes(f) || !called {
		t.Error("func rule should pass and be invoked")
	}
	if r.Message() != "nope" {
		t.Errorf("Message()=%q", r.Message())
	}
}

func TestFieldBasics(t *testing.T) {
	f := &field{name: "c", rv: reflect.ValueOf(1), ctx: context.Background()}
	if f.Name() != "c" || f.Val().Interface() != 1 {
		t.Error("name/val accessors")
	}
	if f.Context() == nil {
		t.Error("Context() must never be nil")
	}
	if _, ok := f.Sibling("x"); ok {
		t.Error("Sibling with no lookup must report false")
	}

	// Sibling resolves via the data source
	vd := &validation{src: mapSource{m: map[string]any{"other": 42}}}
	g := &field{name: "g", scope: "g", vd: vd}
	if s, ok := g.Sibling("other"); !ok || s.Val().Interface() != 42 {
		t.Error("Sibling should resolve via the source")
	}
}
