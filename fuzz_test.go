package validator

import (
	"strings"
	"testing"

	"github.com/libtnb/validator/internal/dsl"
)

// FuzzParse asserts parse/compile never panic on arbitrary input: succeed or return *dsl.ParseError.
func FuzzParse(f *testing.F) {
	seeds := []string{
		"",
		"required",
		"a && b",
		"a||b",
		"!(a && b)",
		"in:a,b,c",
		`regex:"^(a|b)+$"`,
		`regex:^\d+$ && required`,
		"a && (b || !c)",
		"  spaced   &&   x ",
		`in:a\,b,c`,
		"&& ||",
		"((((",
		`regex:"unterminated`,
		strings.Repeat("(", 60) + "a" + strings.Repeat(")", 60),
		strings.Repeat("!", 60) + "a",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	permissive := func(sig string) (Rule, ErrorRule, bool) {
		return passRule{sig: sig, pass: true}, nil, true
	}

	f.Fuzz(func(t *testing.T, s string) {
		node, err := dsl.Parse(s, nil)
		if err != nil {
			if _, ok := err.(*dsl.ParseError); !ok {
				t.Fatalf("dsl.Parse(%q) returned non-dsl.ParseError %T: %v", s, err, err)
			}
			return
		}
		if node == nil {
			t.Fatalf("dsl.Parse(%q) returned nil node with nil error", s)
		}
		if _, err := compile(node, permissive); err != nil {
			if _, ok := err.(*dsl.ParseError); !ok {
				t.Fatalf("compile after dsl.Parse(%q) returned non-dsl.ParseError %T: %v", s, err, err)
			}
		}
	})
}
