package validator

import (
	"reflect"
	"testing"
)

func TestCrossNumericBlankSibling(t *testing.T) {
	gt := &gtFieldRule{}
	blank := fakeField{val: reflect.ValueOf(5), attrs: []string{"other"}, siblings: map[string]Field{"other": fakeField{val: reflect.ValueOf("")}}}
	if gt.Passes(blank) {
		t.Error("gtfield with blank-string sibling should fail (not comparable)")
	}
	real := fakeField{val: reflect.ValueOf(5), attrs: []string{"other"}, siblings: map[string]Field{"other": fakeField{val: reflect.ValueOf(3)}}}
	if !gt.Passes(real) {
		t.Error("gtfield 5 > 3 should pass")
	}
	// numeric zero sibling is comparable, not empty
	zero := fakeField{val: reflect.ValueOf(5), attrs: []string{"other"}, siblings: map[string]Field{"other": fakeField{val: reflect.ValueOf(0)}}}
	if !gt.Passes(zero) {
		t.Error("gtfield 5 > 0 should pass (numeric zero sibling is comparable)")
	}
}

// sib builds a siblings map for cross-field tests.
func sib(pairs map[string]any) map[string]Field {
	m := make(map[string]Field, len(pairs))
	for k, v := range pairs {
		rv := reflect.Value{}
		if v != nil {
			rv = reflect.ValueOf(v)
		}
		m[k] = fakeField{val: rv}
	}
	return m
}

func TestRequiredIf(t *testing.T) {
	r := &requiredIfRule{}
	cases := []struct {
		name string
		f    fakeField
		want bool
	}{
		{"trigger absent fails", fakeField{val: reflect.Value{}, attrs: []string{"type", "card"}, siblings: sib(map[string]any{"type": "card"})}, false},
		{"trigger present passes", fakeField{val: reflect.ValueOf("x"), attrs: []string{"type", "card"}, siblings: sib(map[string]any{"type": "card"})}, true},
		{"no trigger empty passes", fakeField{val: reflect.ValueOf(""), attrs: []string{"type", "card"}, siblings: sib(map[string]any{"type": "cash"})}, true},
		{"other missing passes", fakeField{val: reflect.ValueOf(""), attrs: []string{"type", "card"}}, true},
		{"too few args passes", fakeField{val: reflect.ValueOf(""), attrs: []string{"type"}}, true},
	}
	for _, c := range cases {
		if got := r.Passes(c.f); got != c.want {
			t.Errorf("%s: Passes()=%v want %v", c.name, got, c.want)
		}
	}
	if r.Signature() != "required_if" {
		t.Errorf("Signature()=%q", r.Signature())
	}
}

func TestRequiredUnless(t *testing.T) {
	r := &requiredUnlessRule{}
	cases := []struct {
		name string
		f    fakeField
		want bool
	}{
		{"not equal absent fails", fakeField{val: reflect.Value{}, attrs: []string{"type", "admin"}, siblings: sib(map[string]any{"type": "user"})}, false},
		{"not equal present passes", fakeField{val: reflect.ValueOf("x"), attrs: []string{"type", "admin"}, siblings: sib(map[string]any{"type": "user"})}, true},
		{"equal empty passes", fakeField{val: reflect.ValueOf(""), attrs: []string{"type", "admin"}, siblings: sib(map[string]any{"type": "admin"})}, true},
		{"other missing absent fails", fakeField{val: reflect.Value{}, attrs: []string{"type", "admin"}}, false},
		{"too few args passes", fakeField{val: reflect.ValueOf(""), attrs: []string{"type"}}, true},
	}
	for _, c := range cases {
		if got := r.Passes(c.f); got != c.want {
			t.Errorf("%s: Passes()=%v want %v", c.name, got, c.want)
		}
	}
	if r.Signature() != "required_unless" {
		t.Errorf("Signature()=%q", r.Signature())
	}
}

func TestRequiredWith(t *testing.T) {
	r := &requiredWithRule{}
	cases := []struct {
		name string
		f    fakeField
		want bool
	}{
		{"sibling present self absent fails", fakeField{val: reflect.Value{}, attrs: []string{"a", "b"}, siblings: sib(map[string]any{"a": "v", "b": ""})}, false},
		{"sibling present self present passes", fakeField{val: reflect.ValueOf("x"), attrs: []string{"a"}, siblings: sib(map[string]any{"a": "v"})}, true},
		{"all siblings empty passes", fakeField{val: reflect.ValueOf(""), attrs: []string{"a", "b"}, siblings: sib(map[string]any{"a": "", "b": ""})}, true},
		{"siblings missing passes", fakeField{val: reflect.ValueOf(""), attrs: []string{"a"}}, true},
		{"no args passes", fakeField{val: reflect.ValueOf("")}, true},
	}
	for _, c := range cases {
		if got := r.Passes(c.f); got != c.want {
			t.Errorf("%s: Passes()=%v want %v", c.name, got, c.want)
		}
	}
	if r.Signature() != "required_with" {
		t.Errorf("Signature()=%q", r.Signature())
	}
}

func TestRequiredWithout(t *testing.T) {
	r := &requiredWithoutRule{}
	cases := []struct {
		name string
		f    fakeField
		want bool
	}{
		{"sibling empty self absent fails", fakeField{val: reflect.Value{}, attrs: []string{"a"}, siblings: sib(map[string]any{"a": ""})}, false},
		{"sibling missing self absent fails", fakeField{val: reflect.Value{}, attrs: []string{"a"}}, false},
		{"sibling empty self present passes", fakeField{val: reflect.ValueOf("x"), attrs: []string{"a"}, siblings: sib(map[string]any{"a": ""})}, true},
		{"all siblings present passes", fakeField{val: reflect.ValueOf(""), attrs: []string{"a", "b"}, siblings: sib(map[string]any{"a": "v", "b": "w"})}, true},
		{"one missing self absent fails", fakeField{val: reflect.Value{}, attrs: []string{"a", "b"}, siblings: sib(map[string]any{"a": "v"})}, false},
		{"no args passes", fakeField{val: reflect.ValueOf("")}, true},
	}
	for _, c := range cases {
		if got := r.Passes(c.f); got != c.want {
			t.Errorf("%s: Passes()=%v want %v", c.name, got, c.want)
		}
	}
	if r.Signature() != "required_without" {
		t.Errorf("Signature()=%q", r.Signature())
	}
}

func TestSame(t *testing.T) {
	r := &sameRule{}
	cases := []struct {
		name string
		f    fakeField
		want bool
	}{
		{"equal passes", fakeField{val: reflect.ValueOf("x"), attrs: []string{"o"}, siblings: sib(map[string]any{"o": "x"})}, true},
		{"numeric coerced equal passes", fakeField{val: reflect.ValueOf(1), attrs: []string{"o"}, siblings: sib(map[string]any{"o": "1"})}, true},
		{"not equal fails", fakeField{val: reflect.ValueOf("x"), attrs: []string{"o"}, siblings: sib(map[string]any{"o": "y"})}, false},
		{"empty self passes", fakeField{val: reflect.ValueOf(""), attrs: []string{"o"}, siblings: sib(map[string]any{"o": "y"})}, true},
		{"missing sibling fails", fakeField{val: reflect.ValueOf("x"), attrs: []string{"o"}}, false},
		{"no args fails", fakeField{val: reflect.ValueOf("x")}, false},
	}
	for _, c := range cases {
		if got := r.Passes(c.f); got != c.want {
			t.Errorf("%s: Passes()=%v want %v", c.name, got, c.want)
		}
	}
	if r.Signature() != "same" {
		t.Errorf("Signature()=%q", r.Signature())
	}
}

func TestDifferent(t *testing.T) {
	r := &differentRule{}
	cases := []struct {
		name string
		f    fakeField
		want bool
	}{
		{"different passes", fakeField{val: reflect.ValueOf("x"), attrs: []string{"o"}, siblings: sib(map[string]any{"o": "y"})}, true},
		{"same fails", fakeField{val: reflect.ValueOf("x"), attrs: []string{"o"}, siblings: sib(map[string]any{"o": "x"})}, false},
		{"empty self passes", fakeField{val: reflect.ValueOf(""), attrs: []string{"o"}, siblings: sib(map[string]any{"o": ""})}, true},
		{"missing sibling passes", fakeField{val: reflect.ValueOf("x"), attrs: []string{"o"}}, true},
		{"no args fails", fakeField{val: reflect.ValueOf("x")}, false},
	}
	for _, c := range cases {
		if got := r.Passes(c.f); got != c.want {
			t.Errorf("%s: Passes()=%v want %v", c.name, got, c.want)
		}
	}
	if r.Signature() != "different" {
		t.Errorf("Signature()=%q", r.Signature())
	}
}

func TestEqField(t *testing.T) {
	r := &eqFieldRule{}
	if !r.Passes(fakeField{val: reflect.ValueOf("x"), attrs: []string{"o"}, siblings: sib(map[string]any{"o": "x"})}) {
		t.Error("equal should pass")
	}
	if r.Passes(fakeField{val: reflect.ValueOf("x"), attrs: []string{"o"}, siblings: sib(map[string]any{"o": "y"})}) {
		t.Error("not equal should fail")
	}
	if !r.Passes(fakeField{val: reflect.ValueOf(""), attrs: []string{"o"}}) {
		t.Error("empty self should pass")
	}
	if r.Passes(fakeField{val: reflect.ValueOf("x"), attrs: []string{"o"}}) {
		t.Error("missing sibling should fail")
	}
	if r.Signature() != "eqfield" {
		t.Errorf("Signature()=%q", r.Signature())
	}
}

func TestNeField(t *testing.T) {
	r := &neFieldRule{}
	if !r.Passes(fakeField{val: reflect.ValueOf("x"), attrs: []string{"o"}, siblings: sib(map[string]any{"o": "y"})}) {
		t.Error("different should pass")
	}
	if r.Passes(fakeField{val: reflect.ValueOf("x"), attrs: []string{"o"}, siblings: sib(map[string]any{"o": "x"})}) {
		t.Error("same should fail")
	}
	if !r.Passes(fakeField{val: reflect.ValueOf(""), attrs: []string{"o"}, siblings: sib(map[string]any{"o": ""})}) {
		t.Error("empty self should pass")
	}
	if !r.Passes(fakeField{val: reflect.ValueOf("x"), attrs: []string{"o"}}) {
		t.Error("missing sibling should pass")
	}
	if r.Signature() != "nefield" {
		t.Errorf("Signature()=%q", r.Signature())
	}
}

func TestGtField(t *testing.T) {
	r := &gtFieldRule{}
	cases := []struct {
		name string
		f    fakeField
		want bool
	}{
		{"greater passes", fakeField{val: reflect.ValueOf(5), attrs: []string{"o"}, siblings: sib(map[string]any{"o": 3})}, true},
		{"equal fails", fakeField{val: reflect.ValueOf(5), attrs: []string{"o"}, siblings: sib(map[string]any{"o": 5})}, false},
		{"less fails", fakeField{val: reflect.ValueOf(2), attrs: []string{"o"}, siblings: sib(map[string]any{"o": 5})}, false},
		{"numeric string passes", fakeField{val: reflect.ValueOf("5"), attrs: []string{"o"}, siblings: sib(map[string]any{"o": "3"})}, true},
		{"non numeric other fails", fakeField{val: reflect.ValueOf(5), attrs: []string{"o"}, siblings: sib(map[string]any{"o": "abc"})}, false},
		{"empty self passes", fakeField{val: reflect.ValueOf(""), attrs: []string{"o"}, siblings: sib(map[string]any{"o": 3})}, true},
		{"missing sibling fails", fakeField{val: reflect.ValueOf(5), attrs: []string{"o"}}, false},
		{"no args fails", fakeField{val: reflect.ValueOf(5)}, false},
	}
	for _, c := range cases {
		if got := r.Passes(c.f); got != c.want {
			t.Errorf("%s: Passes()=%v want %v", c.name, got, c.want)
		}
	}
	if r.Signature() != "gtfield" {
		t.Errorf("Signature()=%q", r.Signature())
	}
}

func TestGteField(t *testing.T) {
	r := &gteFieldRule{}
	if !r.Passes(fakeField{val: reflect.ValueOf(5), attrs: []string{"o"}, siblings: sib(map[string]any{"o": 5})}) {
		t.Error("equal should pass")
	}
	if !r.Passes(fakeField{val: reflect.ValueOf(6), attrs: []string{"o"}, siblings: sib(map[string]any{"o": 5})}) {
		t.Error("greater should pass")
	}
	if r.Passes(fakeField{val: reflect.ValueOf(4), attrs: []string{"o"}, siblings: sib(map[string]any{"o": 5})}) {
		t.Error("less should fail")
	}
	if r.Signature() != "gtefield" {
		t.Errorf("Signature()=%q", r.Signature())
	}
}

func TestLtField(t *testing.T) {
	r := &ltFieldRule{}
	if !r.Passes(fakeField{val: reflect.ValueOf(3), attrs: []string{"o"}, siblings: sib(map[string]any{"o": 5})}) {
		t.Error("less should pass")
	}
	if r.Passes(fakeField{val: reflect.ValueOf(5), attrs: []string{"o"}, siblings: sib(map[string]any{"o": 5})}) {
		t.Error("equal should fail")
	}
	if r.Passes(fakeField{val: reflect.ValueOf(7), attrs: []string{"o"}, siblings: sib(map[string]any{"o": 5})}) {
		t.Error("greater should fail")
	}
	if r.Signature() != "ltfield" {
		t.Errorf("Signature()=%q", r.Signature())
	}
}

func TestLteField(t *testing.T) {
	r := &lteFieldRule{}
	if !r.Passes(fakeField{val: reflect.ValueOf(5), attrs: []string{"o"}, siblings: sib(map[string]any{"o": 5})}) {
		t.Error("equal should pass")
	}
	if !r.Passes(fakeField{val: reflect.ValueOf(4), attrs: []string{"o"}, siblings: sib(map[string]any{"o": 5})}) {
		t.Error("less should pass")
	}
	if r.Passes(fakeField{val: reflect.ValueOf(6), attrs: []string{"o"}, siblings: sib(map[string]any{"o": 5})}) {
		t.Error("greater should fail")
	}
	if r.Signature() != "ltefield" {
		t.Errorf("Signature()=%q", r.Signature())
	}
}

func TestConfirmed(t *testing.T) {
	r := &confirmedRule{}
	cases := []struct {
		name string
		f    fakeField
		want bool
	}{
		{"match passes", fakeField{name: "password", val: reflect.ValueOf("x"), siblings: sib(map[string]any{"password_confirmation": "x"})}, true},
		{"mismatch fails", fakeField{name: "password", val: reflect.ValueOf("x"), siblings: sib(map[string]any{"password_confirmation": "y"})}, false},
		{"empty self passes", fakeField{name: "password", val: reflect.ValueOf(""), siblings: sib(map[string]any{"password_confirmation": "y"})}, true},
		{"missing confirmation fails", fakeField{name: "password", val: reflect.ValueOf("x")}, false},
	}
	for _, c := range cases {
		if got := r.Passes(c.f); got != c.want {
			t.Errorf("%s: Passes()=%v want %v", c.name, got, c.want)
		}
	}
	if r.Signature() != "confirmed" {
		t.Errorf("Signature()=%q", r.Signature())
	}
}

// TestCrossfieldContracts checks every rule has a non-empty signature and message.
func TestCrossfieldContracts(t *testing.T) {
	rs := []Rule{
		&requiredIfRule{}, &requiredUnlessRule{}, &requiredWithRule{}, &requiredWithoutRule{},
		&sameRule{}, &differentRule{}, &eqFieldRule{}, &neFieldRule{},
		&gtFieldRule{}, &gteFieldRule{}, &ltFieldRule{}, &lteFieldRule{}, &confirmedRule{},
	}
	for _, r := range rs {
		if r.Signature() == "" {
			t.Errorf("%T: empty signature", r)
		}
		if r.Message() == "" {
			t.Errorf("%T: empty message", r)
		}
	}
}
