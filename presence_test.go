package validator

import "testing"

func TestRequiredDefault(t *testing.T) {
	r := &requiredRule{}
	if r.Signature() != "required" {
		t.Errorf("Signature()=%q, want %q", r.Signature(), "required")
	}
	// Default: present & non-nil passes, even zero values.
	tests := []struct {
		name string
		val  any
		want bool
	}{
		{"non-empty string", "x", true},
		{"empty string (present)", "", true},
		{"zero int (present)", 0, true},
		{"false bool (present)", false, true},
		{"empty slice (present)", []int{}, true},
		{"nil", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := r.Passes(fakeField{val: rvOf(tt.val)}); got != tt.want {
				t.Errorf("Passes(%#v)=%v, want %v", tt.val, got, tt.want)
			}
		})
	}
}

func TestRequiredStrict(t *testing.T) {
	r := &requiredRule{strict: true}
	// Strict: zero values fail.
	tests := []struct {
		val  any
		want bool
	}{
		{"x", true},
		{1, true},
		{"", false},
		{0, false},
		{false, false},
		{nil, false},
		{[]int{}, false},
	}
	for _, tt := range tests {
		if got := r.Passes(fakeField{val: rvOf(tt.val)}); got != tt.want {
			t.Errorf("strict Passes(%#v)=%v, want %v", tt.val, got, tt.want)
		}
	}
}

func TestFilled(t *testing.T) {
	r := &filledRule{}
	if r.Signature() != "filled" {
		t.Errorf("Signature()=%q, want %q", r.Signature(), "filled")
	}
	tests := []struct {
		name string
		val  any
		want bool
	}{
		{"non-empty string", "x", true},
		{"non-zero int", 5, true},
		{"non-empty slice", []string{"a"}, true},
		{"empty string", "", false},
		{"zero int", 0, false},
		{"nil", nil, false},
		{"empty slice", []string{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := r.Passes(fakeField{val: rvOf(tt.val)}); got != tt.want {
				t.Errorf("Passes(%#v)=%v, want %v", tt.val, got, tt.want)
			}
		})
	}
}

func TestNotBlank(t *testing.T) {
	r := &notBlankRule{}
	if r.Signature() != "notblank" {
		t.Errorf("Signature()=%q, want %q", r.Signature(), "notblank")
	}
	tests := []struct {
		name string
		val  any
		want bool
	}{
		{"non-blank string", "x", true},
		{"padded non-blank", "  x  ", true},
		{"numeric stringifies", 1, true},
		{"empty string", "", false},
		{"spaces only", "   ", false},
		{"tab and newline only", "\t\n", false},
		{"nil stringifies empty", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := r.Passes(fakeField{val: rvOf(tt.val)}); got != tt.want {
				t.Errorf("Passes(%#v)=%v, want %v", tt.val, got, tt.want)
			}
		})
	}
}
