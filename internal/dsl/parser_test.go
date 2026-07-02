package dsl

import (
	"strings"
	"testing"
)

// TestParseOperatorChainBounded: a huge || chain fails fast (maxNodes), no stack overflow.
func TestParseOperatorChainBounded(t *testing.T) {
	expr := "a" + strings.Repeat(" || a", maxNodes+10)
	if _, err := Parse(expr, nil); err == nil {
		t.Error("an oversized operator chain should return an error, not overflow")
	}
}

// sexpr renders an AST as an S-expression for easy structure/precedence asserts.
func sexpr(n Node) string {
	switch nd := n.(type) {
	case Leaf:
		if len(nd.Args) == 0 {
			return nd.Name
		}
		return nd.Name + ":" + strings.Join(nd.Args, ",")
	case Not:
		return "(! " + sexpr(nd.X) + ")"
	case And:
		return "(&& " + sexpr(nd.L) + " " + sexpr(nd.R) + ")"
	case Or:
		return "(|| " + sexpr(nd.L) + " " + sexpr(nd.R) + ")"
	default:
		return "?"
	}
}

func TestParsePrecedence(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"a", "a"},
		{"(a)", "a"},
		{"a && b", "(&& a b)"},
		{"a || b && c", "(|| a (&& b c))"}, // && tighter than ||
		{"a && b || c", "(|| (&& a b) c)"},
		{"!a && b", "(&& (! a) b)"}, // ! tighter than &&
		{"!(a || b)", "(! (|| a b))"},
		{"a && b && c", "(&& (&& a b) c)"}, // left-associative
		{"a || b || c", "(|| (|| a b) c)"},
		{"in:a,b || regex:\"x\"", "(|| in:a,b regex:x)"},
	}
	for _, c := range cases {
		node, err := Parse(c.in, nil)
		if err != nil {
			t.Errorf("Parse(%q) error: %v", c.in, err)
			continue
		}
		if got := sexpr(node); got != c.want {
			t.Errorf("Parse(%q) = %s want %s", c.in, got, c.want)
		}
	}
}

func TestParseErrors(t *testing.T) {
	for _, in := range []string{"", "a &&", "&& a", "(a", "a b", "a || ", "!", "()"} {
		if _, err := Parse(in, nil); err == nil {
			t.Errorf("Parse(%q) should have failed", in)
		} else if pe, ok := err.(*ParseError); !ok {
			t.Errorf("Parse(%q) error type = %T want *ParseError", in, err)
		} else if msg := pe.Error(); !strings.Contains(msg, "dsl:") {
			t.Errorf("Parse(%q) ParseError.Error() = %q, want it to mention %q", in, msg, "dsl:")
		}
	}
}

// TestParseDeepNestingFailsGracefully: nesting past maxNestingDepth returns *ParseError, never overflows the stack.
func TestParseDeepNestingFailsGracefully(t *testing.T) {
	deepParen := strings.Repeat("(", maxNestingDepth+10) + "a" + strings.Repeat(")", maxNestingDepth+10)
	if _, err := Parse(deepParen, nil); err == nil {
		t.Error("deeply nested parens should return an error, not overflow the stack")
	}
	deepNot := strings.Repeat("!", maxNestingDepth+10) + "a"
	if _, err := Parse(deepNot, nil); err == nil {
		t.Error("deeply nested ! should return an error")
	}
	ok := strings.Repeat("(", 50) + "a" + strings.Repeat(")", 50)
	if _, err := Parse(ok, nil); err != nil {
		t.Errorf("depth-50 nesting should parse fine: %v", err)
	}
}
