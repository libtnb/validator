package validator

import (
	"testing"
)

func TestFilterSignatures(t *testing.T) {
	cases := []struct {
		filter Filter
		want   string
	}{
		{&trimFilter{}, "trim"},
		{&ltrimFilter{}, "ltrim"},
		{&rtrimFilter{}, "rtrim"},
		{&lowerFilter{}, "lower"},
		{&upperFilter{}, "upper"},
		{&titleFilter{}, "title"},
		{&intFilter{}, "int"},
		{&floatFilter{}, "float"},
		{&boolFilter{}, "bool"},
		{&stringFilter{}, "string"},
	}
	for _, c := range cases {
		if got := c.filter.Signature(); got != c.want {
			t.Errorf("Signature()=%q, want %q", got, c.want)
		}
	}
}

func TestStringFilters(t *testing.T) {
	cases := []struct {
		name   string
		filter Filter
		val    any
		args   []string
		want   string
	}{
		{"trim spaces", &trimFilter{}, "  x  ", nil, "x"},
		{"trim tabs and newlines", &trimFilter{}, "\t\nhi\n ", nil, "hi"},
		{"trim cutset", &trimFilter{}, "xxhellox", []string{"x"}, "hello"},
		{"trim non-string", &trimFilter{}, 42, nil, "42"},

		{"ltrim spaces", &ltrimFilter{}, "  x  ", nil, "x  "},
		{"ltrim cutset", &ltrimFilter{}, "00123", []string{"0"}, "123"},

		{"rtrim spaces", &rtrimFilter{}, "  x  ", nil, "  x"},
		{"rtrim cutset", &rtrimFilter{}, "12300", []string{"0"}, "123"},

		{"lower", &lowerFilter{}, "HeLLo", nil, "hello"},
		{"lower non-string", &lowerFilter{}, true, nil, "true"},

		{"upper", &upperFilter{}, "HeLLo", nil, "HELLO"},

		{"title two words", &titleFilter{}, "hello world", nil, "Hello World"},
		{"title mixed case", &titleFilter{}, "hELLo WORLD", nil, "Hello World"},
		{"title collapses spaces", &titleFilter{}, "  foo   bar  ", nil, "Foo Bar"},
		{"title empty", &titleFilter{}, "", nil, ""},

		{"string from int", &stringFilter{}, 7, nil, "7"},
		{"string from nil", &stringFilter{}, nil, nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := c.filter.Handle(c.val, c.args...)
			if err != nil {
				t.Fatalf("Handle() unexpected error: %v", err)
			}
			gs, ok := got.(string)
			if !ok {
				t.Fatalf("Handle() returned %T, want string", got)
			}
			if gs != c.want {
				t.Errorf("Handle(%v)=%q, want %q", c.val, gs, c.want)
			}
		})
	}
}

func TestIntFilter(t *testing.T) {
	got, err := (&intFilter{}).Handle("42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != int64(42) {
		t.Errorf("Handle(\"42\")=%v (%T), want int64(42)", got, got)
	}
	got, err = (&intFilter{}).Handle(3.9)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != int64(3) {
		t.Errorf("Handle(3.9)=%v, want int64(3)", got)
	}
	if _, err := (&intFilter{}).Handle("abc"); err == nil {
		t.Error("Handle(\"abc\") expected error, got nil")
	}
}

func TestFloatFilter(t *testing.T) {
	got, err := (&floatFilter{}).Handle("3.14")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 3.14 {
		t.Errorf("Handle(\"3.14\")=%v (%T), want float64(3.14)", got, got)
	}
	if _, err := (&floatFilter{}).Handle("xyz"); err == nil {
		t.Error("Handle(\"xyz\") expected error, got nil")
	}
}

func TestBoolFilter(t *testing.T) {
	cases := []struct {
		val  any
		want bool
	}{
		{"true", true},
		{"on", true},
		{"1", true},
		{"false", false},
		{"off", false},
		{0, false},
		{1, true},
	}
	for _, c := range cases {
		got, err := (&boolFilter{}).Handle(c.val)
		if err != nil {
			t.Fatalf("Handle(%v) unexpected error: %v", c.val, err)
		}
		if got != c.want {
			t.Errorf("Handle(%v)=%v, want %v", c.val, got, c.want)
		}
	}
	if _, err := (&boolFilter{}).Handle("notabool"); err == nil {
		t.Error("Handle(\"notabool\") expected error, got nil")
	}
}
