package validator

import (
	"reflect"
	"testing"
)

func TestDescribeRules(t *testing.T) {
	type Address struct {
		City string `validate:"required && min:2"`
	}
	type User struct {
		Name    string   `validate:"required && notblank && min:3 && max:255"`
		Age     int      `validate:"gte:18"`
		Role    string   `validate:"required && (in:admin,user || eq:root)"`
		Tags    []string `validate:"required && dive && alpha"`
		Address Address
		Plain   string
	}

	fields, err := DescribeRules(User{})
	if err != nil {
		t.Fatalf("DescribeRules: %v", err)
	}

	byName := map[string]FieldRules{}
	for _, f := range fields {
		byName[f.Name] = f
	}

	name := byName["Name"]
	want := []RuleInfo{{Name: "required"}, {Name: "notblank"}, {Name: "min", Args: []string{"3"}}, {Name: "max", Args: []string{"255"}}}
	if !reflect.DeepEqual(name.Rules, want) {
		t.Errorf("Name rules = %+v, want %+v", name.Rules, want)
	}
	if !name.Exact {
		t.Error("Name should flatten exactly")
	}

	if role := byName["Role"]; role.Exact {
		t.Error("Role contains ||, Exact must be false")
	} else if len(role.Rules) != 1 || role.Rules[0].Name != "required" {
		t.Errorf("Role should keep the flat required rule, got %+v", role.Rules)
	}

	tags := byName["Tags"]
	if len(tags.Rules) != 1 || tags.Rules[0].Name != "required" {
		t.Errorf("Tags container rules = %+v", tags.Rules)
	}
	if len(tags.Element) != 1 || tags.Element[0].Name != "alpha" {
		t.Errorf("Tags element rules = %+v", tags.Element)
	}

	city, ok := byName["Address.City"]
	if !ok {
		t.Fatal("nested Address.City should be described")
	}
	typ := reflect.TypeOf(User{})
	if got := typ.FieldByIndex(city.Index).Name; got != "City" {
		t.Errorf("Index should locate the Go field, got %s", got)
	}

	if _, ok := byName["Plain"]; ok {
		t.Error("fields without rules should be omitted")
	}
}

func TestDescribeRules_UnknownRuleTolerated(t *testing.T) {
	type Req struct {
		ID uint `validate:"required && exists:users,id"`
	}

	fields, err := DescribeRules(Req{})
	if err != nil {
		t.Fatalf("unknown rules must not fail introspection: %v", err)
	}
	if len(fields) != 1 || len(fields[0].Rules) != 2 || fields[0].Rules[1].Name != "exists" {
		t.Errorf("unexpected result: %+v", fields)
	}
}

func TestDescribeRules_NonStruct(t *testing.T) {
	if _, err := DescribeRules(42); err == nil {
		t.Error("non-struct input should error")
	}
}
