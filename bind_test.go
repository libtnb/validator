package validator

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestBindAndSafeBind(t *testing.T) {
	type User struct {
		Email string `validate:"required && email"`
		Age   int    `validate:"required && numeric"`
	}

	t.Run("SafeBind after passing validation", func(t *testing.T) {
		vd := Map(
			map[string]any{"Email": "a@b.com", "Age": "42"},
			map[string]string{"Email": "required && email", "Age": "required && numeric"},
		)
		vd.Validate(context.Background())
		var u User
		if err := vd.SafeBind(&u); err != nil {
			t.Fatalf("SafeBind: %v", err)
		}
		if u.Email != "a@b.com" || u.Age != 42 {
			t.Errorf("bound = %+v", u)
		}
	})

	t.Run("SafeBind before Validate errors", func(t *testing.T) {
		vd := Map(map[string]any{"Email": "a@b.com"}, map[string]string{"Email": "required"})
		var u User
		if err := vd.SafeBind(&u); err == nil {
			t.Error("SafeBind before Validate should error")
		}
	})

	t.Run("SafeBind on failed validation errors", func(t *testing.T) {
		vd := Map(map[string]any{"Email": "nope"}, map[string]string{"Email": "required && email"})
		vd.Validate(context.Background())
		var u User
		if err := vd.SafeBind(&u); err == nil {
			t.Error("SafeBind on failure should error")
		}
	})

	t.Run("Bind ignores validation state", func(t *testing.T) {
		vd := Map(map[string]any{"Email": "raw", "Age": 7}, map[string]string{"Email": "required"})
		var u User
		if err := vd.Bind(&u); err != nil {
			t.Fatalf("Bind: %v", err)
		}
		if u.Email != "raw" || u.Age != 7 {
			t.Errorf("bound = %+v", u)
		}
	})

	t.Run("non-pointer target errors", func(t *testing.T) {
		vd := Map(map[string]any{}, map[string]string{})
		var u User
		if err := vd.Bind(u); err == nil {
			t.Error("Bind to non-pointer should error")
		}
	})

	t.Run("nested struct with nil pointer allocates", func(t *testing.T) {
		type Inner struct {
			City string `validate:"required"`
		}
		type Outer struct {
			Name  string `validate:"required"`
			Inner *Inner // nil target; bindInto must allocate, not panic
		}
		vd := Map(
			map[string]any{"Name": "x", "Inner.City": "NYC"},
			map[string]string{"Name": "required"},
		)
		var out Outer
		if err := vd.Bind(&out); err != nil {
			t.Fatalf("Bind: %v", err)
		}
		if out.Name != "x" {
			t.Errorf("Name = %q", out.Name)
		}
		if out.Inner == nil || out.Inner.City != "NYC" {
			t.Errorf("nested pointer not allocated/bound: %+v", out.Inner)
		}
	})
}

func TestVarAndPackageFuncs(t *testing.T) {
	ok := Var("a@b.com", "required && email")
	ok.Validate(context.Background())
	if ok.Fails() {
		t.Errorf("valid var should pass: %v", ok.Errors().All())
	}

	bad := Var("nope", "required && email")
	bad.Validate(context.Background())
	if !bad.Fails() {
		t.Error("invalid var should fail")
	}

	type T struct {
		Name string `validate:"required"`
	}
	sv := NewValidator(WithStrictRequired()).Struct(T{Name: ""})
	sv.Validate(context.Background())
	if !sv.Fails() {
		t.Error("empty required name should fail under strict required")
	}
}

func TestBindPointerScalar(t *testing.T) {
	type T struct {
		Age  *int
		Name *string
	}
	vd := Map(map[string]any{"Age": "42", "Name": "bob"}, map[string]string{})
	var out T
	if err := vd.Bind(&out); err != nil {
		t.Fatal(err)
	}
	if out.Age == nil || *out.Age != 42 {
		t.Errorf("pointer int bind = %v", out.Age)
	}
	if out.Name == nil || *out.Name != "bob" {
		t.Errorf("pointer string bind = %v", out.Name)
	}
}

func TestBindUntaggedFromStructSource(t *testing.T) {
	type Src struct {
		Email string `validate:"email"`
		Name  string
	}
	type Dst struct {
		Email string
		Name  string
	}
	vd := Struct(Src{Email: "a@b.com", Name: "bob"})
	vd.Validate(context.Background())
	var d Dst
	if err := vd.Bind(&d); err != nil {
		t.Fatal(err)
	}
	if d.Name != "bob" {
		t.Errorf("untagged Name should bind from a struct source, got %q", d.Name)
	}
}

func TestBindUntaggedFields(t *testing.T) {
	type User struct {
		Email string `validate:"required && email"`
		Name  string // untagged — must still bind
	}
	vd := Map(
		map[string]any{"Email": "a@b.com", "Name": "bob"},
		map[string]string{"Email": "required && email"},
	)
	vd.Validate(context.Background())
	var u User
	if err := vd.SafeBind(&u); err != nil {
		t.Fatal(err)
	}
	if u.Name != "bob" {
		t.Errorf("untagged Name should be bound, got %q", u.Name)
	}
}

func TestBindIntegerOverflowSkipped(t *testing.T) {
	type T struct {
		N int8
	}
	vd := Map(map[string]any{"N": "300"}, map[string]string{})
	var out T
	if err := vd.Bind(&out); err != nil {
		t.Fatal(err)
	}
	if out.N != 0 {
		t.Errorf("overflowing 300 into int8 should be skipped (0), got %d (wrapped)", out.N)
	}
}

func TestVarBindDoesNotFanValue(t *testing.T) {
	type User struct {
		Email string
		Age   int
		Name  string
	}
	vd := Var("7", "numeric")
	vd.Validate(context.Background())
	var u User
	_ = vd.SafeBind(&u)
	if u.Email != "" || u.Age != 0 || u.Name != "" {
		t.Errorf("Var SafeBind must not fan the scalar into struct fields, got %+v", u)
	}
}

// Recursive pointer field must not stack-overflow (pointer recursion is depth-bounded).
func TestBindRecursivePointerNoCrash(t *testing.T) {
	completesWithin(t, "Bind into recursive ptr field", func() {
		type P *P
		type S struct {
			A P
			B string
		}
		var out S
		_ = Map(map[string]any{"A": "x", "B": "y"}, nil).Bind(&out)
	})
}

// binding a JSON number/bool into a time.Time field must not stack-overflow.
func TestToTimeBindNoCrash(t *testing.T) {
	type T struct{ When time.Time }
	completesWithin(t, "Bind float64 into time.Time", func() {
		var out T
		_ = Map(map[string]any{"When": 1700000000.0}, nil).Bind(&out)
	})
	completesWithin(t, "Bind bool into time.Time", func() {
		var out T
		_ = Map(map[string]any{"When": true}, nil).Bind(&out)
	})
	completesWithin(t, "JSON timestamp number bind", func() {
		var out T
		vd := JSON(`{"When": 1700000000}`, map[string]string{})
		vd.Validate(context.Background())
		_ = vd.Bind(&out)
	})
	// a real date string still binds
	var out T
	if err := Map(map[string]any{"When": "2024-01-02T15:04:05Z"}, nil).Bind(&out); err != nil {
		t.Fatalf("bind date string: %v", err)
	}
	if out.When.IsZero() {
		t.Error("a valid date string should bind into time.Time")
	}
}

// Map bind: total failure keeps preset (writes nothing); empty sets empty; partial keeps good ones.
func TestMapBindNoClobberOnTotalFailure(t *testing.T) {
	type T struct{ M map[string]int }

	// total failure -> preset kept
	preset := T{M: map[string]int{"keep": 9}}
	if err := Map(map[string]any{"M": map[string]any{"a": "notanint"}}, nil).Bind(&preset); err != nil {
		t.Fatal(err)
	}
	if preset.M == nil || preset.M["keep"] != 9 {
		t.Errorf("a total-failure map conversion must not clobber the preset field, got %v", preset.M)
	}

	// partial success -> good entries kept
	var partial T
	if err := Map(map[string]any{"M": map[string]any{"a": 1, "b": "bad"}}, nil).Bind(&partial); err != nil {
		t.Fatal(err)
	}
	if partial.M["a"] != 1 || len(partial.M) != 1 {
		t.Errorf("partial map conversion should keep the good entries, got %v", partial.M)
	}

	// empty source -> empty result
	empty := T{M: map[string]int{"keep": 9}}
	if err := Map(map[string]any{"M": map[string]any{}}, nil).Bind(&empty); err != nil {
		t.Fatal(err)
	}
	if len(empty.M) != 0 {
		t.Errorf("binding an empty map should set empty, got %v", empty.M)
	}
}

// A typed-nil composite source (slice/map) is unwrapped to nil and must not clobber a preset field.
func TestBindTypedNilCompositeNoClobber(t *testing.T) {
	type Src struct {
		S []string
		M map[string]int
		P *int
	}
	type Dst struct {
		S []string
		M map[string]int
		P *int
	}
	n := 5
	preset := Dst{S: []string{"keep"}, M: map[string]int{"k": 1}, P: &n}
	if err := Struct(Src{}).Bind(&preset); err != nil {
		t.Fatal(err)
	}
	if len(preset.S) != 1 || preset.S[0] != "keep" {
		t.Errorf("typed-nil slice source must not clobber the preset slice, got %v", preset.S)
	}
	if len(preset.M) != 1 || preset.M["k"] != 1 {
		t.Errorf("typed-nil map source must not clobber the preset map, got %v", preset.M)
	}
	if preset.P == nil || *preset.P != 5 {
		t.Errorf("nil pointer source must not clobber the preset pointer, got %v", preset.P)
	}
}

// A uint64 above MaxInt64 must bind, not be lost to the signed path.
func TestBindLargeUint64(t *testing.T) {
	type T struct{ U uint64 }
	var out T
	if err := Map(map[string]any{"U": "9223372036854775808"}, nil).Bind(&out); err != nil {
		t.Fatal(err)
	}
	if out.U != 9223372036854775808 {
		t.Errorf("a uint64 above MaxInt64 must bind, got %d", out.U)
	}
}

// setReflect binds slices, maps, and time.Time across type gaps.
func TestBindCompositeAndTime(t *testing.T) {
	type T struct {
		Tags  []string
		Codes map[string]string
		When  time.Time
		WhenP *time.Time
	}
	vd := Map(map[string]any{
		"Tags":  []any{"a", "b", "c"},
		"Codes": map[string]any{"k": "v"},
		"When":  "2024-01-02T03:04:05Z",
		"WhenP": "2024-01-02T03:04:05Z",
	}, map[string]string{})
	var out T
	if err := vd.Bind(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.Tags) != 3 || out.Tags[0] != "a" || out.Tags[2] != "c" {
		t.Errorf("[]any -> []string bind failed: %v", out.Tags)
	}
	if out.Codes["k"] != "v" {
		t.Errorf("map[string]any -> map[string]string bind failed: %v", out.Codes)
	}
	if out.When.IsZero() || out.When.Year() != 2024 {
		t.Errorf("string -> time.Time bind failed: %v", out.When)
	}
	if out.WhenP == nil || out.WhenP.Year() != 2024 {
		t.Errorf("string -> *time.Time bind failed: %v", out.WhenP)
	}
}

// dotted names descend into nested map/JSON values (validation + bind).
func TestNestedJSONLookupAndBind(t *testing.T) {
	vd := JSON(`{"Inner":{"City":"LA"}}`, map[string]string{"Inner.City": "required && alpha"})
	vd.Validate(context.Background())
	if vd.Errors().Has("Inner.City") {
		t.Errorf("nested JSON Inner.City should resolve and pass, got %v", vd.Errors().All())
	}
	type Inner struct{ City string }
	type Outer struct{ Inner Inner }
	var o Outer
	if err := vd.Bind(&o); err != nil {
		t.Fatal(err)
	}
	if o.Inner.City != "LA" {
		t.Errorf("nested JSON should bind into the nested struct, got %q", o.Inner.City)
	}
}

// A failed map-key conversion must not fabricate a phantom zero key.
func TestBindMapTypedKeyNoPhantom(t *testing.T) {
	type T struct{ M map[int]string }
	vd := Map(map[string]any{"M": map[string]any{"bad": "x", "1": "one"}}, map[string]string{})
	var out T
	if err := vd.Bind(&out); err != nil {
		t.Fatal(err)
	}
	if _, has0 := out.M[0]; has0 {
		t.Errorf("a failed key must not fabricate a 0 entry: %v", out.M)
	}
	if out.M[1] != "one" {
		t.Errorf("the valid key should bind: %v", out.M)
	}
}

// A failed slice element stays zero in place; dropping it would shift later indices.
func TestBindSliceFailedElementKeepsPosition(t *testing.T) {
	type T struct{ Nums []int }
	vd := Map(map[string]any{"Nums": []any{1, "x", 3}}, map[string]string{})
	var out T
	if err := vd.Bind(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.Nums) != 3 || out.Nums[0] != 1 || out.Nums[1] != 0 || out.Nums[2] != 3 {
		t.Errorf("a failed element must keep its position as zero, got %v", out.Nums)
	}
}

// slice-of-struct binds element-wise from nested objects.
func TestBindSliceOfStruct(t *testing.T) {
	type Inner struct{ City string }
	type T struct{ Items []Inner }
	vd := Map(map[string]any{"Items": []any{
		map[string]any{"City": "NYC"},
		map[string]any{"City": "LA"},
	}}, map[string]string{})
	var out T
	if err := vd.Bind(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.Items) != 2 || out.Items[0].City != "NYC" || out.Items[1].City != "LA" {
		t.Errorf("slice-of-struct bind failed: %+v", out.Items)
	}
}

// a failed conversion must not overwrite a preset pointer with &zero.
func TestBindPointerPreservedOnFailure(t *testing.T) {
	type T struct{ Ptr *int }
	pre := 99
	out := T{Ptr: &pre}
	vd := Map(map[string]any{"Ptr": "notanint"}, map[string]string{})
	if err := vd.Bind(&out); err != nil {
		t.Fatal(err)
	}
	if out.Ptr == nil || *out.Ptr != 99 {
		t.Errorf("a failed conversion must leave the pointer untouched (was 99), got %v", out.Ptr)
	}
}

// float32 overflow is skipped, not written as +Inf.
func TestBindFloat32Overflow(t *testing.T) {
	type T struct{ F float32 }
	out := T{}
	vd := Map(map[string]any{"F": "1e100"}, map[string]string{})
	if err := vd.Bind(&out); err != nil {
		t.Fatal(err)
	}
	if out.F != 0 {
		t.Errorf("float32 overflow must be skipped (stay 0), got %v", out.F)
	}
}

// []byte / []rune fields bind from a string (incl. json.RawMessage).
func TestBindByteRuneSliceFromString(t *testing.T) {
	type T struct {
		B []byte
		R []rune
		J json.RawMessage
	}
	vd := Map(map[string]any{"B": "hello", "R": "world", "J": `{"a":1}`}, map[string]string{})
	var out T
	if err := vd.Bind(&out); err != nil {
		t.Fatal(err)
	}
	if string(out.B) != "hello" {
		t.Errorf("[]byte from string = %q", out.B)
	}
	if string(out.R) != "world" {
		t.Errorf("[]rune from string = %q", string(out.R))
	}
	if string(out.J) != `{"a":1}` {
		t.Errorf("json.RawMessage from string = %q", out.J)
	}
}

// validate:"-" skips validation but keeps the field bindable and cross-field-resolvable.
func TestDashKeepsFieldBindableAndResolvable(t *testing.T) {
	type T struct {
		Email    string `validate:"required && email"`
		Internal string `validate:"-"`
	}
	vd := Struct(T{Email: "a@b.com", Internal: "keep-me"})
	vd.Validate(context.Background())
	if vd.Fails() {
		t.Errorf(`validate:"-" field must not be validated: %v`, vd.Errors().All())
	}
	var out T
	if err := vd.Bind(&out); err != nil {
		t.Fatal(err)
	}
	if out.Internal != "keep-me" {
		t.Errorf(`validate:"-" field must still bind (no data loss), got %q`, out.Internal)
	}

	// cross-field rules resolve a "-" field
	type F struct {
		Secret string `validate:"-"`
		Echo   string `validate:"same:Secret"`
	}
	good := Struct(F{Secret: "x", Echo: "x"})
	good.Validate(context.Background())
	if good.Fails() {
		t.Errorf(`same:Secret should resolve a "-" field, got %v`, good.Errors().All())
	}
	mism := Struct(F{Secret: "x", Echo: "y"})
	mism.Validate(context.Background())
	if !mism.Fails() {
		t.Error("mismatched same:Secret should fail")
	}
}

// Bind dereferences pointer ELEMENTS of slices/maps across a type gap.
func TestBindPointerElements(t *testing.T) {
	a, b := 5, 6
	type T struct {
		S []int64
		M map[string]int64
	}
	vd := Map(map[string]any{
		"S": []*int{&a, &b},
		"M": map[string]*int{"x": &a},
	}, map[string]string{})
	var out T
	if err := vd.Bind(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.S) != 2 || out.S[0] != 5 || out.S[1] != 6 {
		t.Errorf("[]*int -> []int64 must deref elements, got %v", out.S)
	}
	if out.M["x"] != 5 {
		t.Errorf("map[string]*int -> map[string]int64 must deref values, got %v", out.M)
	}
}

// a slice binds positionally — a nil/failed element keeps its slot.
func TestBindSlicePositionalWithNil(t *testing.T) {
	a, c := 1, 3
	type T struct{ P []*int64 }
	vd := Map(map[string]any{"P": []*int{&a, nil, &c}}, map[string]string{})
	var out T
	if err := vd.Bind(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.P) != 3 {
		t.Fatalf("positional length must be 3, got %d (%v)", len(out.P), out.P)
	}
	if out.P[0] == nil || *out.P[0] != 1 {
		t.Errorf("P[0] = %v, want 1", out.P[0])
	}
	if out.P[1] != nil {
		t.Errorf("P[1] (nil source) must stay nil, got %v", out.P[1])
	}
	if out.P[2] == nil || *out.P[2] != 3 {
		t.Errorf("P[2] = %v, want 3", out.P[2])
	}
}

// A failed leaf conversion allocates no ghost intermediate pointers on the target.
func TestBindNoGhostPointersOnFailure(t *testing.T) {
	type Inner struct{ N int }
	type T struct{ In *Inner }
	vd := Map(map[string]any{"In.N": "not-a-number"}, map[string]string{})
	var out T
	if err := vd.Bind(&out); err != nil {
		t.Fatal(err)
	}
	if out.In != nil {
		t.Errorf("a failed leaf conversion must not fabricate the intermediate pointer, got %+v", out.In)
	}

	// the success path still allocates the chain
	ok := Map(map[string]any{"In": map[string]any{"N": "42"}}, map[string]string{})
	var out2 T
	if err := ok.Bind(&out2); err != nil {
		t.Fatal(err)
	}
	if out2.In == nil || out2.In.N != 42 {
		t.Errorf("successful nested bind should allocate and set, got %+v", out2.In)
	}
}

// Colliding converted map keys have a deterministic survivor.
func TestBindMapKeyCollisionDeterministic(t *testing.T) {
	type T struct{ M map[string]string }
	src := map[any]any{1: "int-one", "1": "str-one"}
	var want string
	for i := range 20 {
		vd := Map(map[string]any{"M": src}, map[string]string{})
		var out T
		if err := vd.Bind(&out); err != nil {
			t.Fatal(err)
		}
		got := out.M["1"]
		if got == "" {
			t.Fatalf("expected a surviving entry for key \"1\", got %v", out.M)
		}
		if i == 0 {
			want = got
			continue
		}
		if got != want {
			t.Fatalf("nondeterministic collision survivor: run %d got %q want %q", i, got, want)
		}
	}
}
