package validator

import "testing"

func TestAlpha(t *testing.T) {
	r := &alphaRule{}
	if r.Signature() != "alpha" {
		t.Errorf("Signature()=%q", r.Signature())
	}
	cases := []struct {
		val  any
		want bool
	}{
		{"abcXYZ", true},
		{"héllo", true}, // unicode letters
		{"", true},
		{nil, true},
		{"abc123", false},
		{"abc def", false},
		{"abc!", false},
	}
	for _, c := range cases {
		if got := r.Passes(fakeField{val: rvOf(c.val)}); got != c.want {
			t.Errorf("Alpha.Passes(%v)=%v want %v", c.val, got, c.want)
		}
	}
}

func TestAlphanum(t *testing.T) {
	r := &alphanumRule{}
	if r.Signature() != "alphanum" {
		t.Errorf("Signature()=%q", r.Signature())
	}
	cases := []struct {
		val  any
		want bool
	}{
		{"abc123", true},
		{"ABC", true},
		{"123", true},
		{1234, true}, // numeric via conv
		{"", true},
		{nil, true},
		{"abc 123", false},
		{"abc-123", false},
		{"abc!", false},
	}
	for _, c := range cases {
		if got := r.Passes(fakeField{val: rvOf(c.val)}); got != c.want {
			t.Errorf("Alphanum.Passes(%v)=%v want %v", c.val, got, c.want)
		}
	}
}

func TestAscii(t *testing.T) {
	r := &asciiRule{}
	if r.Signature() != "ascii" {
		t.Errorf("Signature()=%q", r.Signature())
	}
	cases := []struct {
		val  any
		want bool
	}{
		{"abc123!@# ", true},
		{"~\x7f", true}, // 0x7f (127) is ASCII
		{"", true},
		{nil, true},
		{"héllo", false},
		{"日本語", false},
	}
	for _, c := range cases {
		if got := r.Passes(fakeField{val: rvOf(c.val)}); got != c.want {
			t.Errorf("Ascii.Passes(%v)=%v want %v", c.val, got, c.want)
		}
	}
}

func TestLowercase(t *testing.T) {
	r := &lowercaseRule{}
	if r.Signature() != "lowercase" {
		t.Errorf("Signature()=%q", r.Signature())
	}
	cases := []struct {
		val  any
		want bool
	}{
		{"abc def", true},
		{"abc123", true}, // digits have no case
		{"", true},
		{nil, true},
		{"Abc", false},
		{"ABC", false},
	}
	for _, c := range cases {
		if got := r.Passes(fakeField{val: rvOf(c.val)}); got != c.want {
			t.Errorf("Lowercase.Passes(%v)=%v want %v", c.val, got, c.want)
		}
	}
}

func TestUppercase(t *testing.T) {
	r := &uppercaseRule{}
	if r.Signature() != "uppercase" {
		t.Errorf("Signature()=%q", r.Signature())
	}
	cases := []struct {
		val  any
		want bool
	}{
		{"ABC DEF", true},
		{"ABC123", true}, // digits have no case
		{"", true},
		{nil, true},
		{"Abc", false},
		{"abc", false},
	}
	for _, c := range cases {
		if got := r.Passes(fakeField{val: rvOf(c.val)}); got != c.want {
			t.Errorf("Uppercase.Passes(%v)=%v want %v", c.val, got, c.want)
		}
	}
}

func TestContains(t *testing.T) {
	r := &containsRule{}
	if r.Signature() != "contains" {
		t.Errorf("Signature()=%q", r.Signature())
	}
	cases := []struct {
		val   any
		attrs []string
		want  bool
	}{
		{"hello world", []string{"world"}, true},
		{"hello world", []string{"xyz"}, false},
		{"", []string{"x"}, true},
		{nil, []string{"x"}, true},
		{"hello", nil, true}, // no arg -> pass
		{"hello", []string{}, true},
		{"hello", []string{"hello"}, true},
	}
	for _, c := range cases {
		if got := r.Passes(fakeField{val: rvOf(c.val), attrs: c.attrs}); got != c.want {
			t.Errorf("Contains.Passes(%v, %v)=%v want %v", c.val, c.attrs, got, c.want)
		}
	}
}

func TestExcludes(t *testing.T) {
	r := &excludesRule{}
	if r.Signature() != "excludes" {
		t.Errorf("Signature()=%q", r.Signature())
	}
	cases := []struct {
		val   any
		attrs []string
		want  bool
	}{
		{"hello world", []string{"xyz"}, true},
		{"hello world", []string{"world"}, false},
		{"", []string{"x"}, true},
		{nil, []string{"x"}, true},
		{"hello", nil, true}, // no arg -> pass
		{"hello", []string{}, true},
	}
	for _, c := range cases {
		if got := r.Passes(fakeField{val: rvOf(c.val), attrs: c.attrs}); got != c.want {
			t.Errorf("Excludes.Passes(%v, %v)=%v want %v", c.val, c.attrs, got, c.want)
		}
	}
}

func TestStartsWith(t *testing.T) {
	r := &startsWithRule{}
	if r.Signature() != "startswith" {
		t.Errorf("Signature()=%q", r.Signature())
	}
	cases := []struct {
		val   any
		attrs []string
		want  bool
	}{
		{"hello world", []string{"hello"}, true},
		{"hello world", []string{"world"}, false},
		{"", []string{"x"}, true},
		{nil, []string{"x"}, true},
		{"hello", nil, true}, // no arg -> pass
		{"hello", []string{}, true},
	}
	for _, c := range cases {
		if got := r.Passes(fakeField{val: rvOf(c.val), attrs: c.attrs}); got != c.want {
			t.Errorf("StartsWith.Passes(%v, %v)=%v want %v", c.val, c.attrs, got, c.want)
		}
	}
}

func TestEndsWith(t *testing.T) {
	r := &endsWithRule{}
	if r.Signature() != "endswith" {
		t.Errorf("Signature()=%q", r.Signature())
	}
	cases := []struct {
		val   any
		attrs []string
		want  bool
	}{
		{"hello world", []string{"world"}, true},
		{"hello world", []string{"hello"}, false},
		{"", []string{"x"}, true},
		{nil, []string{"x"}, true},
		{"hello", nil, true}, // no arg -> pass
		{"hello", []string{}, true},
	}
	for _, c := range cases {
		if got := r.Passes(fakeField{val: rvOf(c.val), attrs: c.attrs}); got != c.want {
			t.Errorf("EndsWith.Passes(%v, %v)=%v want %v", c.val, c.attrs, got, c.want)
		}
	}
}
