package validator

import (
	"reflect"
	"testing"
)

func TestUnique(t *testing.T) {
	r := &uniqueRule{}
	cases := []struct {
		val  any
		want bool
	}{
		{[]string{"a", "b", "c"}, true},
		{[]string{"a", "b", "a"}, false},
		{[]int{1, 2, 3}, true},
		{[]int{1, 2, 1}, false},
		// mixed dynamic types stay distinct: 1 != "1"
		{[]any{1, "1"}, true},
		{[]any{1, 1}, false},
		// pointers compare by pointee
		{[]*int{ptrTo(1), ptrTo(2)}, true},
		{[]*int{ptrTo(1), ptrTo(1)}, false},
		// map values
		{map[string]string{"a": "x", "b": "y"}, true},
		{map[string]string{"a": "x", "b": "x"}, false},
		// empty passes (omitempty)
		{[]string{}, true},
		{nil, true},
		// non-collection fails closed
		{"abc", false},
	}
	for _, c := range cases {
		if got := r.Passes(fakeField{val: reflect.ValueOf(c.val)}); got != c.want {
			t.Errorf("unique(%v)=%v want %v", c.val, got, c.want)
		}
	}
}

func ptrTo[T any](v T) *T { return &v }
