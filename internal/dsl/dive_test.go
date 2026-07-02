package dsl

import "testing"

func TestSplitDiveMalformed(t *testing.T) {
	for _, in := range []string{
		"|| dive && a",
		"a && dive ||",
		"a dive b",
		"a && b dive c",
		"required && dive", // trailing dive: would silently disable element rules
		"dive",
	} {
		if _, _, _, err := SplitDive(in, nil); err == nil {
			t.Errorf("SplitDive(%q) should error on malformed dive placement", in)
		}
	}
	for _, c := range []struct {
		in        string
		container string
		element   string
	}{
		{"required && dive && email", "required", "email"},
		{"dive && email", "", "email"},
		{"in:a,b && dive && email", "in:a,b", "email"},
	} {
		container, element, has, err := SplitDive(c.in, nil)
		if err != nil || !has || container != c.container || element != c.element {
			t.Errorf("SplitDive(%q) = (%q,%q,%v,%v) want (%q,%q,true,nil)", c.in, container, element, has, err, c.container, c.element)
		}
	}
}

// dive is a structural separator: an || across it is meaningless and rejected.
func TestSplitDiveRejectsOrConnector(t *testing.T) {
	for _, in := range []string{
		"a || dive && b",
		"a && dive || b",
		"required || dive",
		"dive || email",
	} {
		if _, _, _, err := SplitDive(in, nil); err == nil {
			t.Errorf("SplitDive(%q) should reject an || connector around dive", in)
		}
	}
	container, element, has, err := SplitDive("required && dive && email", nil)
	if err != nil || !has || container != "required" || element != "email" {
		t.Errorf("SplitDive(&&-form) = (%q,%q,%v,%v)", container, element, has, err)
	}
}

// ContainsDiveToken finds a standalone dive leaf at any paren depth.
func TestContainsDiveToken(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"dive", true},
		{"dive && email", true},
		{"(dive && email)", true},
		{"email && (a || dive)", true},
		{"required && email", false},
		{"diver", false},  // not standalone
		{"dive:x", false}, // arg-carrying dive is not the keyword
		{"", false},
	}
	for _, c := range cases {
		if got := ContainsDiveToken(c.in, nil); got != c.want {
			t.Errorf("ContainsDiveToken(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestHasTopLevelOr(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"a || b", true},
		{"a && b", false},
		{"a", false},
		{"(a || b)", false},      // nested
		{"(a || b) && c", false}, // top level is &&
		{"a && (b || c)", false}, // nested
		{"a || (b && c)", true},
		{"a && b || c", true},
		{`regex:"a|b" && c`, false}, // | in quoted arg is not an operator
		{"", false},
	}
	for _, c := range cases {
		if got := HasTopLevelOr(c.in, nil); got != c.want {
			t.Errorf("HasTopLevelOr(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// RemoveTopLevelLeaves drops named top-level leaves; the reserved dive separator is never removed.
func TestRemoveTopLevelLeaves(t *testing.T) {
	cases := []struct {
		in    string
		names map[string]bool
		want  string
	}{
		{"required && alpha && email", map[string]bool{"email": true}, "required && alpha"},
		{"required && alpha", map[string]bool{"required": true}, "alpha"},
		{"required && alpha", map[string]bool{"alpha": true}, "required"},
		{"required", map[string]bool{"required": true}, ""},
		{"required && alpha", map[string]bool{"zzz": true}, "required && alpha"},                  // absent: no-op
		{"required && dive && email", map[string]bool{"dive": true}, "required && dive && email"}, // dive reserved
	}
	for _, c := range cases {
		if got := RemoveTopLevelLeaves(c.in, c.names, nil); got != c.want {
			t.Errorf("RemoveTopLevelLeaves(%q, %v) = %q, want %q", c.in, c.names, got, c.want)
		}
	}
}
