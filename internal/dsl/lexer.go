package dsl

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// ParseError is a lexing/parsing failure with its position.
type ParseError struct {
	Pos int
	Msg string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("dsl: %s at position %d", e.Msg, e.Pos)
}

// IsRawArgFunc reports whether a rule reads its argument raw (one arg, commas literal).
type IsRawArgFunc func(name string) bool

func DefaultIsRawArg(name string) bool {
	return name == "regex" || name == "not_regex"
}

// HasTopLevelOr reports whether input has a `||` outside parentheses.
// Lex error yields true: safe to wrap a malformed expr before &&-joining.
func HasTopLevelOr(input string, isRawArg IsRawArgFunc) bool {
	toks, err := lex(input, isRawArg)
	if err != nil {
		return true
	}
	depth := 0
	for k := range toks {
		switch toks[k].typ {
		case tLParen:
			depth++
		case tRParen:
			if depth > 0 {
				depth--
			}
		case tOr:
			if depth == 0 {
				return true
			}
		}
	}
	return false
}

// lex tokenizes a rule expression. Operators must be doubled (&& ||); a lone
// & or | errors, so a single | inside a regex is never mistaken for OR.
func lex(input string, isRawArg IsRawArgFunc) ([]token, error) {
	if isRawArg == nil {
		isRawArg = DefaultIsRawArg
	}
	var toks []token
	i, n := 0, len(input)
	for i < n {
		c := input[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '(':
			toks = append(toks, token{typ: tLParen, pos: i})
			i++
		case c == ')':
			toks = append(toks, token{typ: tRParen, pos: i})
			i++
		case c == '!':
			toks = append(toks, token{typ: tNot, pos: i})
			i++
		case c == '&':
			if i+1 < n && input[i+1] == '&' {
				toks = append(toks, token{typ: tAnd, pos: i})
				i += 2
			} else {
				return nil, &ParseError{Pos: i, Msg: "unexpected '&', did you mean '&&'"}
			}
		case c == '|':
			if i+1 < n && input[i+1] == '|' {
				toks = append(toks, token{typ: tOr, pos: i})
				i += 2
			} else {
				return nil, &ParseError{Pos: i, Msg: "unexpected '|', did you mean '||'"}
			}
		case isIdentStart(c):
			start := i
			for i < n && isIdentChar(input[i]) {
				i++
			}
			name := input[start:i]
			var args []string
			if i < n && input[i] == ':' {
				i++
				var err error
				args, i, err = readArgs(input, i, isRawArg(name))
				if err != nil {
					return nil, err
				}
			}
			toks = append(toks, token{typ: tLeaf, name: name, args: args, pos: start})
		default:
			r, _ := utf8.DecodeRuneInString(input[i:])
			return nil, &ParseError{Pos: i, Msg: fmt.Sprintf("unexpected character %q", string(r))}
		}
	}
	toks = append(toks, token{typ: tEOF, pos: n})
	return toks, nil
}

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentChar(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}

// readArgs reads args after ':': raw rules yield one (commas literal), else
// split on top-level commas. Edge whitespace is trimmed only when it was lexed
// OUTSIDE quotes: quoted content survives verbatim (only \" and \\ escape).
func readArgs(input string, i int, raw bool) ([]string, int, error) {
	n := len(input)

	if raw {
		var b strings.Builder
		i, protected, err := scanArg(input, i, n, &b)
		if err != nil {
			return nil, i, err
		}
		return []string{trimUnquoted(b.String(), protected)}, i, nil
	}

	var args []string
	var b strings.Builder
	protected := 0 // builder length up to the end of the last quoted chunk
	for i < n {
		c := input[i]
		if c == ')' || isOp(input, i, '&') || isOp(input, i, '|') {
			break
		}
		if c == '&' || c == '|' {
			// fail-fast: a lone & or | is never silently part of an argument
			return nil, i, &ParseError{Pos: i, Msg: fmt.Sprintf("unexpected %q in argument, escape it as %q or quote the argument", string(c), "\\"+string(c))}
		}
		if c == ',' {
			args = append(args, trimUnquoted(b.String(), protected))
			b.Reset()
			protected = 0
			i++
			continue
		}
		if b.Len() == 0 && protected == 0 && isSpaceByte(c) {
			i++ // leading unquoted whitespace
			continue
		}
		var err error
		i, protected, err = scanChunk(input, i, n, &b, protected)
		if err != nil {
			return nil, i, err
		}
	}
	args = append(args, trimUnquoted(b.String(), protected))
	return args, i, nil
}

// scanArg reads one raw argument verbatim (regex bodies keep a bare | or &;
// only doubled operators or ')' terminate).
func scanArg(input string, i, n int, b *strings.Builder) (int, int, error) {
	protected := 0
	for i < n {
		if input[i] == ')' || isOp(input, i, '&') || isOp(input, i, '|') {
			break
		}
		if b.Len() == 0 && protected == 0 && isSpaceByte(input[i]) {
			i++
			continue
		}
		var err error
		i, protected, err = scanChunk(input, i, n, b, protected)
		if err != nil {
			return i, protected, err
		}
	}
	return i, protected, nil
}

func isSpaceByte(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }

// trimUnquoted trims trailing whitespace not covered by a quoted chunk.
func trimUnquoted(s string, protected int) string {
	end := len(s)
	for end > protected && isSpaceByte(s[end-1]) {
		end--
	}
	return s[:end]
}

// scanChunk consumes one chunk into b, returning the new protected watermark
// (builder length after a quoted chunk: quote-covered content is never
// trimmed). Two escape regimes (see isEscapable / isQuoteEscapable): other
// backslashes stay literal so regex \d \w \s survive.
func scanChunk(input string, i, n int, b *strings.Builder, protected int) (int, int, error) {
	c := input[i]
	if c == '\\' && i+1 < n {
		if nc := input[i+1]; isEscapable(nc) {
			b.WriteByte(nc)
			return i + 2, protected, nil
		}
		b.WriteByte(c) // literal backslash
		return i + 1, protected, nil
	}
	if c == '"' {
		open := i
		j := i + 1
		for j < n {
			if input[j] == '\\' && j+1 < n && isQuoteEscapable(input[j+1]) {
				b.WriteByte(input[j+1])
				j += 2
				continue
			}
			if input[j] == '"' {
				return j + 1, b.Len(), nil
			}
			b.WriteByte(input[j])
			j++
		}
		return j, protected, &ParseError{Pos: open, Msg: "unterminated quote in argument"}
	}
	b.WriteByte(c)
	return i + 1, protected, nil
}

func isEscapable(c byte) bool {
	switch c {
	case '\\', '"', '(', ')', '&', '|', ',':
		return true
	default:
		return false
	}
}

func isQuoteEscapable(c byte) bool {
	return c == '"' || c == '\\'
}

func isOp(input string, i int, ch byte) bool {
	return input[i] == ch && i+1 < len(input) && input[i+1] == ch
}
