package validator

import (
	"context"
	"reflect"
	"testing"
)

func TestStructValidate(t *testing.T) {
	type User struct {
		Email string `validate:"required && email"`
		Age   int    `validate:"required && gte:18"`
		Bio   string `validate:"-"`
		Name  string // no tag
	}
	v := NewValidator()

	t.Run("valid", func(t *testing.T) {
		vd := v.Struct(User{Email: "a@b.com", Age: 20})
		vd.Validate(context.Background())
		if vd.Fails() {
			t.Errorf("valid user should pass: %v", vd.Errors().All())
		}
	})

	t.Run("invalid collects field errors", func(t *testing.T) {
		vd := v.Struct(User{Email: "nope", Age: 10})
		vd.Validate(context.Background())
		if !vd.Fails() {
			t.Fatal("invalid user should fail")
		}
		if !vd.Errors().Has("Email") || !vd.Errors().Has("Age") {
			t.Errorf("expected Email and Age errors, got %v", vd.Errors().All())
		}
	})

	t.Run("pointer to struct", func(t *testing.T) {
		vd := v.Struct(&User{Email: "a@b.com", Age: 20})
		vd.Validate(context.Background())
		if vd.Fails() {
			t.Errorf("pointer struct should validate: %v", vd.Errors().All())
		}
	})
}

// TestStructPlanCached verifies the per-type plan is built once and reused.
func TestStructPlanCached(t *testing.T) {
	type T struct {
		A string `validate:"required"`
	}
	v := NewValidator()
	p1 := v.getStructPlan(reflect.TypeFor[T]())
	p2 := v.getStructPlan(reflect.TypeFor[T]())
	if p1 != p2 {
		t.Error("struct plan should be cached and identical across calls")
	}
}

// A validate tag on an embedded struct field is a value rule under the type
// name, like a named struct field (a nil embedded pointer must fail required).
func TestEmbeddedFieldTagHonored(t *testing.T) {
	type Config struct {
		Host string
	}
	type App struct {
		*Config `validate:"required"`
	}
	v := NewValidator()
	vd := v.Struct(App{})
	vd.Validate(context.Background())
	if !vd.Errors().Has("Config") {
		t.Errorf("required on a nil embedded pointer must fail, got %v", vd.Errors().All())
	}
	ok := v.Struct(App{Config: &Config{Host: "h"}})
	ok.Validate(context.Background())
	if ok.Fails() {
		t.Errorf("present embedded pointer should pass, got %v", ok.Errors().All())
	}
}

// Untagged struct-typed fields stay resolvable by name: cross-field rules and
// AddRules see them, matching the Map source on identical logical data.
func TestUntaggedStructFieldResolvable(t *testing.T) {
	type Card struct {
		Number string
	}
	type Payment struct {
		Method *string `validate:"required_with:Card"`
		Card   *Card
	}
	v := NewValidator()
	vd := v.Struct(Payment{Card: &Card{Number: "4111"}})
	vd.Validate(context.Background())
	if !vd.Errors().Has("Method") {
		t.Errorf("required_with must see the untagged struct field Card, got %v", vd.Errors().All())
	}

	type Holder struct {
		Card *Card
	}
	hv := v.Struct(Holder{Card: &Card{Number: "1"}})
	if err := hv.AddRules("Card", "required"); err != nil {
		t.Fatal(err)
	}
	hv.Validate(context.Background())
	if hv.Fails() {
		t.Errorf("AddRules(required) on a present struct field must pass, got %v", hv.Errors().All())
	}
}

// Struct types with no reachable subfields (named time types and the like) are
// plan leaves: bindable, resolvable, and rule-addressable.
func TestOpaqueStructFieldIsLeaf(t *testing.T) {
	type MyTime struct{ sec int64 }
	type Rec struct {
		At MyTime
	}
	v := NewValidator()
	vd := v.Struct(Rec{At: MyTime{sec: 5}})
	if err := vd.AddRules("At", "required"); err != nil {
		t.Fatal(err)
	}
	vd.Validate(context.Background())
	if vd.Fails() {
		t.Errorf("required on a present opaque struct must pass, got %v", vd.Errors().All())
	}

	src := Rec{At: MyTime{sec: 7}}
	var dst Rec
	if err := v.Struct(src).Bind(&dst); err != nil {
		t.Fatal(err)
	}
	if dst.At != src.At {
		t.Errorf("Bind must not drop an opaque struct field: got %+v want %+v", dst.At, src.At)
	}
}

// A recursion-cut self-referential field is still a bindable leaf.
func TestRecursiveStructFieldBindable(t *testing.T) {
	type Node struct {
		Val  string
		Next *Node
	}
	v := NewValidator()
	src := Node{Val: "a", Next: &Node{Val: "b"}}
	var dst Node
	if err := v.Struct(src).Bind(&dst); err != nil {
		t.Fatal(err)
	}
	if dst.Next == nil || dst.Next.Val != "b" {
		t.Errorf("Bind must whole-assign a recursion-cut field, got %+v", dst.Next)
	}
}

// A shadowed embedded subtree leaves no ghost entries: rules on Go-unreachable
// paths never run and Bind never writes into the shadowed field.
func TestShadowedSubtreeNoGhosts(t *testing.T) {
	type hiddenX struct {
		OnlyA string `validate:"required"`
	}
	type base struct {
		X hiddenX
	}
	type Outer struct {
		base
		X string `validate:"required"`
	}
	v := NewValidator()
	vd := v.Struct(Outer{X: "visible"})
	vd.Validate(context.Background())
	if vd.Errors().Has("X.OnlyA") {
		t.Errorf("rules on a shadowed subtree path must not run, got %v", vd.Errors().All())
	}
	if vd.Fails() {
		t.Errorf("only the visible X is validated, got %v", vd.Errors().All())
	}
}

// prune must not read through its own compaction: a dropped (ambiguous) entry
// ordered before a nested struct's children must not shift what isGhost reads,
// silently dropping legitimate nested fields.
func TestPruneKeepsNestedFieldsAfterDrop(t *testing.T) {
	type embA2 struct {
		X string `validate:"notblank"`
	}
	type embB2 struct {
		X string `validate:"numeric"`
	}
	type inner2 struct {
		N1 string `validate:"notblank"`
		N2 string `validate:"notblank"`
	}
	type top struct {
		embA2
		embB2 // X is ambiguous -> dead entry BEFORE the nested children
		Outer inner2
	}
	v := NewValidator()
	vd := v.Struct(top{})
	vd.Validate(context.Background())
	if !vd.Errors().Has("Outer.N1") || !vd.Errors().Has("Outer.N2") {
		t.Errorf("both nested fields must survive prune and validate, got %v", vd.Errors().All())
	}

	src := top{Outer: inner2{N1: "a", N2: "b"}}
	var dst top
	if err := v.Struct(src).Bind(&dst); err != nil {
		t.Fatal(err)
	}
	if dst.Outer.N1 != "a" || dst.Outer.N2 != "b" {
		t.Errorf("Bind must not lose nested fields after a prune drop, got %+v", dst.Outer)
	}
}

// Equal-depth same-name promotions are ambiguous, matching Go: neither field's
// rules run under the promoted name.
func TestAmbiguousPromotionDropped(t *testing.T) {
	type embA struct {
		Code string `validate:"notblank"`
	}
	type embB struct {
		Code string `validate:"numeric"`
	}
	type Both struct {
		embA
		embB
	}
	v := NewValidator()
	vd := v.Struct(Both{})
	vd.Validate(context.Background())
	if vd.Errors().Has("Code") {
		t.Errorf("an ambiguous promoted name must validate under neither rule set, got %v", vd.Errors().All())
	}

	// a shallower unambiguous field still wins over deeper ambiguity
	type Shallow struct {
		embA
		embB
		Code string `validate:"notblank"`
	}
	sv := v.Struct(Shallow{})
	sv.Validate(context.Background())
	if !sv.Errors().Has("Code") {
		t.Errorf("the shallow unambiguous Code must validate, got %v", sv.Errors().All())
	}
}
