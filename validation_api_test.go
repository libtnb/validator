package validator

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestField_GenericAccessors(t *testing.T) {
	root := map[string]any{"value": 42}
	field := &Field{
		name: "value",
		val:  reflect.ValueOf(42),
		data: root,
		siblings: map[string]*Field{
			"other": {val: reflect.ValueOf("ok")},
		},
	}
	value, ok := field.Value[int]()
	if !ok || value != 42 {
		t.Fatalf("Value = %v, %v", value, ok)
	}
	if _, ok := field.Value[string](); ok {
		t.Fatal("Value accepted the wrong type")
	}
	other, ok := field.Sibling[string]("other")
	if !ok || other != "ok" {
		t.Fatalf("Sibling = %q, %v", other, ok)
	}
	gotRoot, ok := field.Root[map[string]any]()
	if !ok || gotRoot["value"] != 42 {
		t.Fatalf("Root = %v, %v", gotRoot, ok)
	}
}

func TestValidation_MutationAfterValidateFails(t *testing.T) {
	validation := MustMap(map[string]any{"name": "ok"}, map[string]string{"name": "required"})
	if err := validation.Validate(t.Context()); err != nil {
		t.Fatal(err)
	}
	mutations := []func() error{
		func() error { return validation.AddRules("name", "alpha") },
		func() error { return validation.RemoveRules("name", "required") },
		func() error { return validation.ClearRules("name") },
		func() error { return validation.AddFilters("name", "trim") },
		func() error { return validation.RemoveFilters("name", "trim") },
		func() error { return validation.ClearFilters("name") },
		func() error { return validation.AddMessages(map[string]string{"required": "required"}) },
	}
	for i, mutate := range mutations {
		if err := mutate(); !errors.Is(err, ErrValidated) {
			t.Errorf("mutation %d error = %v", i, err)
		}
	}
}

func TestStruct_RejectsNonStruct(t *testing.T) {
	if _, err := Struct(42); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("error = %v, want ErrInvalidInput", err)
	}
}

func TestJSON_StrictSyntax(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "duplicate name", data: []byte(`{"name":"first","name":"second"}`)},
		{name: "invalid utf8", data: []byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := JSON(test.data, nil); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("error = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestBind_RejectsPointerTypeParameter(t *testing.T) {
	type target struct{ Name string }
	var output *target
	err := MustMap(map[string]any{"Name": "value"}, nil).Bind(&output)
	if !errors.Is(err, ErrBindTarget) {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateAs_ReturnsValidationSentinel(t *testing.T) {
	type target struct{ Email string }
	validation := MustMap(
		map[string]any{"Email": "invalid"},
		map[string]string{"Email": "email"},
	)
	var output target
	err := validation.ValidateAs(context.Background(), &output)
	if !errors.Is(err, ErrValidationFailed) {
		t.Fatalf("error = %v", err)
	}
	if output != (target{}) {
		t.Fatalf("failed validation mutated destination: %+v", output)
	}
}

type failingOperationRule struct{ err error }

func (failingOperationRule) Signature() string { return "remote" }
func (failingOperationRule) Message() string   { return "The {field} is unavailable." }
func (r failingOperationRule) Validate(*Field) (bool, error) {
	return false, r.err
}

func TestFallibleRuleOperationalError(t *testing.T) {
	backendErr := errors.New("backend unavailable")
	validator := MustNew(WithoutBuiltinRules(),
		WithFallibleRules(failingOperationRule{err: backendErr}))
	validation := validator.MustValue("x", "remote")
	err := validation.Validate(t.Context())
	if !errors.Is(err, backendErr) || !errors.Is(err, ErrRuleEvaluation) {
		t.Fatalf("error = %v", err)
	}
	var ruleErr *RuleError
	if !errors.As(err, &ruleErr) || ruleErr.Field != "value" || ruleErr.Rule != "remote" {
		t.Fatalf("RuleError = %+v, %v", ruleErr, err)
	}
	if len(validation.Errors().Items()) != 0 {
		t.Fatalf("operational error became field errors: %v", validation.Errors().Items())
	}
}

func TestValidateRejectsNilContextWithoutConsumingValidation(t *testing.T) {
	validation := MustValue("abc", "alpha")
	if err := validation.Validate(nil); !errors.Is(err, ErrNilContext) {
		t.Fatalf("nil context error = %v", err)
	}
	if err := validation.Validate(t.Context()); err != nil {
		t.Fatalf("valid retry after nil context = %v", err)
	}
}
