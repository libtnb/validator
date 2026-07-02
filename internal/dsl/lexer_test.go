package dsl

import (
	"strings"
	"testing"
)

// types returns the token-type sequence, sans trailing EOF.
func types(toks []token) []tokenType {
	out := make([]tokenType, 0, len(toks))
	for _, tk := range toks {
		if tk.typ == tEOF {
			break
		}
		out = append(out, tk.typ)
	}
	return out
}

func eqStrs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestLexUnterminatedQuote(t *testing.T) {
	for _, in := range []string{`regex:"^abc`, `in:"a,b`, `required && regex:"x`} {
		if _, err := Parse(in, nil); err == nil {
			t.Errorf("Parse(%q) should error on an unterminated quote", in)
		}
	}
}

func TestLexLeafArgs(t *testing.T) {
	cases := []struct {
		in   string
		name string
		args []string
	}{
		{"required", "required", nil},
		{"in:a,b,c", "in", []string{"a", "b", "c"}},
		{"in:a, b ,c", "in", []string{"a", "b", "c"}}, // spaces trimmed
		{`in:a\,b,c`, "in", []string{"a,b", "c"}},     // \, stays literal
		{"max:10", "max", []string{"10"}},
	}
	for _, c := range cases {
		toks, err := lex(c.in, nil)
		if err != nil {
			t.Errorf("lex(%q) error: %v", c.in, err)
			continue
		}
		if toks[0].typ != tLeaf || toks[0].name != c.name || !eqStrs(toks[0].args, c.args) {
			t.Errorf("lex(%q) leaf = {%q %v} want {%q %v}", c.in, toks[0].name, toks[0].args, c.name, c.args)
		}
	}
}

func TestLexRegexRawArg(t *testing.T) {
	cases := []struct {
		in  string
		arg string
	}{
		{`regex:"^(a|b)+$"`, "^(a|b)+$"},
		{`regex:^\d+$`, `^\d+$`},
		{`regex:^[a-z|0-9]+$`, "^[a-z|0-9]+$"},
		{`regex:\(\)`, "()"}, // bare: \( \) -> literal ()
		// inside quotes only \" and \\ are escapes; regex metacharacters survive verbatim
		{`regex:"\(\d+\)"`, `\(\d+\)`},
		{`regex:"\d+\w"`, `\d+\w`},
		{`regex:"a\\b"`, `a\b`}, // \\ -> one backslash
		{`regex:"a\"b"`, `a"b`}, // \" -> literal quote
	}
	for _, c := range cases {
		toks, err := lex(c.in, nil)
		if err != nil {
			t.Errorf("lex(%q) error: %v", c.in, err)
			continue
		}
		if toks[0].typ != tLeaf || toks[0].name != "regex" || len(toks[0].args) != 1 || toks[0].args[0] != c.arg {
			t.Errorf("lex(%q) arg = %v want [%q]", c.in, toks[0].args, c.arg)
		}
	}
}

func TestLexRegexThenOperator(t *testing.T) {
	// bare regex stops at top-level &&
	toks, err := lex(`regex:^\d+$ && required`, nil)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	got := types(toks)
	want := []tokenType{tLeaf, tAnd, tLeaf}
	if len(got) != len(want) {
		t.Fatalf("token types = %v want %v", got, want)
	}
	if toks[0].args[0] != `^\d+$` {
		t.Errorf("regex arg = %q want %q", toks[0].args[0], `^\d+$`)
	}
}

func TestLexOperatorsAndGrouping(t *testing.T) {
	toks, err := lex("required && (in:a,b || !empty)", nil)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	got := types(toks)
	want := []tokenType{tLeaf, tAnd, tLParen, tLeaf, tOr, tNot, tLeaf, tRParen}
	if len(got) != len(want) {
		t.Fatalf("token types = %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("token[%d] = %d want %d", i, got[i], want[i])
		}
	}
}

func TestLexSingleOperatorErrors(t *testing.T) {
	for _, in := range []string{"a & b", "a | b", "a @ b"} {
		if _, err := lex(in, nil); err == nil {
			t.Errorf("lex(%q) should have failed", in)
		} else if _, ok := err.(*ParseError); !ok {
			t.Errorf("lex(%q) error type = %T want *ParseError", in, err)
		}
	}
}

// A lone & or | in a bare (non-raw) argument fails fast per the documented
// operator rule; raw args (regex) keep reading them literally.
func TestLexSingleOperatorInArgErrors(t *testing.T) {
	for _, in := range []string{"in:a | in:b", "min:3 & max:5", "contains:a&b"} {
		if _, err := lex(in, nil); err == nil {
			t.Errorf("lex(%q) should fail fast on a lone operator inside a bare argument", in)
		}
	}
	// escaped and quoted forms stay literal
	for _, c := range []struct{ in, arg string }{
		{`contains:a\&b`, "a&b"},
		{`contains:"a|b"`, "a|b"},
		{`regex:^[a|b]+$`, "^[a|b]+$"}, // raw arg: bare | is literal
	} {
		toks, err := lex(c.in, nil)
		if err != nil {
			t.Errorf("lex(%q) error: %v", c.in, err)
			continue
		}
		if len(toks[0].args) != 1 || toks[0].args[0] != c.arg {
			t.Errorf("lex(%q) args = %v want [%q]", c.in, toks[0].args, c.arg)
		}
	}
}

// Quotes protect edge whitespace: only unquoted spacing is trimmed.
func TestLexQuotedEdgeWhitespace(t *testing.T) {
	cases := []struct {
		in  string
		arg string
	}{
		{`eq:" x"`, " x"},
		{`eq:"x "`, "x "},
		{`eq:" "`, " "},
		{`regex:" +"`, " +"},
		{`eq: "x" `, "x"},         // unquoted spacing around the quotes trims
		{`in:a, " b" ,c`, " b"},   // per-arg protection
		{`contains:a" b"`, "a b"}, // mixed bare+quoted keeps the quoted space
	}
	for _, c := range cases {
		toks, err := lex(c.in, nil)
		if err != nil {
			t.Errorf("lex(%q) error: %v", c.in, err)
			continue
		}
		found := false
		for _, a := range toks[0].args {
			if a == c.arg {
				found = true
			}
		}
		if !found {
			t.Errorf("lex(%q) args = %q, want to contain %q", c.in, toks[0].args, c.arg)
		}
	}
}

// A multibyte character in an operator position reports the whole rune.
func TestLexMultibyteErrorMessage(t *testing.T) {
	_, err := lex("a ￥ b", nil)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "￥") {
		t.Errorf("error should name the full rune, got %q", err.Error())
	}
}
