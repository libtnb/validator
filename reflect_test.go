package validator

import (
	"context"
	"testing"
	"time"
	"unsafe"

	"github.com/libtnb/validator/conv"
)

func TestPointerScalarFields(t *testing.T) {
	type T struct {
		Age  *int    `validate:"required && gte:18"`
		Name *string `validate:"required && alpha"`
	}
	age, name := 20, "alice"
	ok := Struct(T{Age: &age, Name: &name})
	ok.Validate(context.Background())
	if ok.Fails() {
		t.Errorf("valid pointer fields should pass: %v", ok.Errors().All())
	}

	// nil pointer is not present, so it fails required
	nilp := Struct(T{})
	nilp.Validate(context.Background())
	if !nilp.Fails() {
		t.Error("nil pointer should fail required")
	}

	// rules see the pointed-to value
	bad := 10
	low := Struct(T{Age: &bad, Name: &name})
	low.Validate(context.Background())
	if !low.Errors().Has("Age") {
		t.Errorf("pointed-to 10 should fail gte:18, got %v", low.Errors().All())
	}
}

// derefType must be bounded: a recursive named pointer field can't loop forever.
func TestDerefTypeRecursivePointerNoHang(t *testing.T) {
	completesWithin(t, "derefType recursive pointer", func() {
		type P *P
		type S struct {
			X P `validate:"required"`
		}
		Struct(S{}).Validate(context.Background())
	})
}

// attachSource must not hang on a top-level recursive pointer (deref is bounded).
func TestSourceOfRecursivePointerNoHang(t *testing.T) {
	completesWithin(t, "Any(recursive ptr)", func() {
		type P *P
		var p P
		p = &p
		Any(p).Validate(context.Background())
	})
	completesWithin(t, "Struct(recursive ptr)", func() {
		type P *P
		var p P
		p = &p
		Struct(p).Validate(context.Background())
	})
}

// Legal deep pointer chains still validate/bind within maxDerefDepth.
func TestLegalPointerChainsStillWork(t *testing.T) {
	type S struct {
		N **int `validate:"required"`
	}
	n := 7
	pn := &n
	v := Struct(S{N: &pn})
	v.Validate(context.Background())
	if v.Fails() {
		t.Errorf("**int should validate within the deref bound, got %v", v.Errors().All())
	}
	var out S
	if err := Map(map[string]any{"N": 5}, nil).Bind(&out); err != nil {
		t.Fatalf("bind **int: %v", err)
	}
	if out.N == nil || *out.N == nil || **out.N != 5 {
		t.Errorf("expected **int bound to 5, got %v", out.N)
	}
}

// A self-referential container must not crash: bounded ToString prevents overflow.
func TestCyclicValueValidationNoCrash(t *testing.T) {
	m := map[string]any{}
	m["self"] = m
	vd := Map(map[string]any{"x": m}, map[string]string{"x": "notblank"})
	vd.Validate(context.Background())
	_ = vd.Fails()

	s := make([]any, 1)
	s[0] = s
	dv := Map(map[string]any{"list": []any{s}}, map[string]string{"list": "dive && notblank"})
	dv.Validate(context.Background())
	_ = dv.Fails()

	dm := Map(map[string]any{"m": map[string]any{"k": s}}, map[string]string{"m": "dive && notblank"})
	dm.Validate(context.Background())
	_ = dm.Fails()
}

// Flat sources (Map/Var) dereference pointer values like the struct path.
func TestFlatSourcePointerDeref(t *testing.T) {
	n := 42
	vd := Map(map[string]any{"age": &n}, map[string]string{"age": "numeric && gte:18"})
	vd.Validate(context.Background())
	if vd.Errors().Has("age") {
		t.Errorf("*int(42) should pass numeric/gte via deref, got %v", vd.Errors().All())
	}

	var np *int
	vd2 := Map(map[string]any{"age": np}, map[string]string{"age": "required"})
	vd2.Validate(context.Background())
	if !vd2.Errors().Has("age") {
		t.Error("a nil *int should fail required (deref to nil), not pass via a non-nil interface")
	}

	vv := Var(&n, "numeric && gte:18")
	vv.Validate(context.Background())
	if vv.Errors().Has("value") {
		t.Errorf("Var(*int) should deref, got %v", vv.Errors().All())
	}
}

// A struct `any` field holding a pointer is dereferenced (Struct/Map parity).
func TestStructAnyFieldDeref(t *testing.T) {
	type S struct {
		Age any `validate:"numeric && gte:18"`
	}
	n := 42
	vd := Struct(S{Age: &n})
	vd.Validate(context.Background())
	if vd.Fails() {
		t.Errorf("an any field holding *int(42) should pass via deref, got %v", vd.Errors().All())
	}
}

// Nil nested map values and nil cross-field siblings must not panic.
func TestNilSafety(t *testing.T) {
	vd := Map(map[string]any{"Addr": nil}, map[string]string{"Addr.City": "required"})
	vd.Validate(context.Background()) // must not panic
	type Out struct {
		Addr struct{ City string }
	}
	var out Out
	if err := vd.Bind(&out); err != nil {
		t.Fatal(err)
	}

	cf := Map(map[string]any{"A": 5, "B": nil}, map[string]string{"A": "gtfield:B"})
	cf.Validate(context.Background())
	_ = cf.Fails()
}

// A pointer/interface-wrapped value validates identically across all entry
// points and inside a dive, since all funnel through unwrap().
func TestDerefConsistencyAcrossEntryPoints(t *testing.T) {
	n := 42
	type SP struct {
		Age *int `validate:"numeric && gte:18"`
	}
	type SA struct {
		Age any `validate:"numeric && gte:18"`
	}
	cases := map[string]Validation{
		"struct *int":      Struct(SP{Age: &n}),
		"struct any->*int": Struct(SA{Age: &n}),
		"map *int":         Map(map[string]any{"Age": &n}, map[string]string{"Age": "numeric && gte:18"}),
		"var *int":         Var(&n, "numeric && gte:18"),
		"dive []*int":      Map(map[string]any{"xs": []*int{&n}}, map[string]string{"xs": "dive && numeric && gte:18"}),
	}
	for name, vd := range cases {
		vd.Validate(context.Background())
		if vd.Fails() {
			t.Errorf("%s: *int(42) should pass via uniform deref, got %v", name, vd.Errors().All())
		}
	}
}

// A self-referential `any` field must not hang getValue (bounded unwrap).
func TestSelfRefAnyFieldNoHang(t *testing.T) {
	type S struct {
		V any `validate:"required"`
	}
	var v any
	v = &v
	done := make(chan bool, 1)
	go func() {
		Struct(S{V: v}).Validate(context.Background())
		done <- true
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("getValue hung on a self-referential any field")
	}
}

// Default required: typed-nil slice/map is absent (like nil pointer); non-nil
// empty slice/map and zero scalar are present.
func TestNilableRequiredConsistency(t *testing.T) {
	type T struct {
		S []string       `validate:"required"`
		M map[string]int `validate:"required"`
	}
	bad := Struct(T{})
	bad.Validate(context.Background())
	if !bad.Errors().Has("S") || !bad.Errors().Has("M") {
		t.Errorf("a nil slice/map should fail required (consistent with nil pointer), got %v", bad.Errors().All())
	}
	ok := Struct(T{S: []string{}, M: map[string]int{}})
	ok.Validate(context.Background())
	if ok.Fails() {
		t.Errorf("a non-nil empty slice/map should pass required, got %v", ok.Errors().All())
	}
	type Z struct {
		X string `validate:"required"`
	}
	zok := Struct(Z{}) // zero string is non-nil-able -> present
	zok.Validate(context.Background())
	if zok.Fails() {
		t.Errorf("a zero string should still pass default required, got %v", zok.Errors().All())
	}
}

type nilStringer struct{ s string }

// conv.ToString must not panic on a nil-pointer Stringer.
func TestNilStringerNoPanic(t *testing.T) {
	var p *nilStringer
	if got := conv.ToString(p); got != "" {
		t.Errorf("ToString(nil Stringer) should be empty, got %q", got)
	}
}

// A nil unsafe.Pointer is absent like every other nil-able kind.
func TestNilUnsafePointerRequired(t *testing.T) {
	type T struct {
		P unsafe.Pointer `validate:"required"`
	}
	vd := Struct(T{})
	vd.Validate(context.Background())
	if !vd.Errors().Has("P") {
		t.Error("required must fail on a nil unsafe.Pointer")
	}
	x := 1
	ok := Struct(T{P: unsafe.Pointer(&x)})
	ok.Validate(context.Background())
	if ok.Fails() {
		t.Errorf("required should pass on a non-nil unsafe.Pointer, got %v", ok.Errors().All())
	}
}

// A nil func must fail required in both default and strict mode.
func TestNilFuncRequired(t *testing.T) {
	var fn func()
	d := Var(fn, "required")
	d.Validate(context.Background())
	if !d.Fails() {
		t.Error("a nil func should fail default required")
	}
	s := NewValidator(WithStrictRequired()).Var(fn, "required")
	s.Validate(context.Background())
	if !s.Fails() {
		t.Error("a nil func should fail strict required (consistent with default)")
	}
	nn := Var(func() {}, "required")
	nn.Validate(context.Background())
	if nn.Fails() {
		t.Error("a non-nil func should pass required")
	}
}
