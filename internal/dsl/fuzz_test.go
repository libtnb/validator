package dsl

import "testing"

// FuzzSplitDive asserts SplitDive never panics, only fails with *ParseError, and that split pieces re-lex cleanly.
func FuzzSplitDive(f *testing.F) {
	for _, s := range []string{
		"required && dive && email",
		"dive && email",
		"required && dive",
		"a || dive && b",
		"a && dive || b",
		"a dive b",
		"(a && dive) && b",
		`regex:"a|b" && dive && required`,
		"a && b && dive && c && d",
		"!a && dive && !b",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		container, element, has, err := SplitDive(s, nil)
		if err != nil {
			if _, ok := err.(*ParseError); !ok {
				t.Fatalf("SplitDive(%q) returned non-ParseError %T: %v", s, err, err)
			}
			return
		}
		if !has {
			if container != s {
				t.Fatalf("SplitDive(%q): no dive but container=%q != input", s, container)
			}
			return
		}
		for _, piece := range []string{container, element} {
			if piece == "" {
				continue
			}
			if _, lerr := lex(piece, nil); lerr != nil {
				t.Fatalf("SplitDive(%q) produced a piece %q that fails to lex: %v", s, piece, lerr)
			}
		}
	})
}
