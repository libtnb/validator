package dsl

import "strings"

// SplitDive splits on a top-level standalone `dive` into container/element exprs.
// Malformed dive placement fails closed.
func SplitDive(input string, isRawArg IsRawArgFunc) (container, element string, hasDive bool, err error) {
	toks, lerr := lex(input, isRawArg)
	if lerr != nil {
		return "", "", false, lerr
	}
	depth := 0
	for k := range toks {
		tk := toks[k]
		switch tk.typ {
		case tLParen:
			depth++
		case tRParen:
			if depth > 0 {
				depth--
			}
		case tLeaf:
			if depth != 0 || tk.name != "dive" || len(tk.args) != 0 {
				continue
			}

			cEnd := tk.pos

			if k > 0 {
				if toks[k-1].typ != tAnd {
					return "", "", false, &ParseError{Pos: tk.pos, Msg: "'dive' must be separated by &&"}
				}
				if k < 2 || !endsValue(toks[k-2].typ) {
					return "", "", false, &ParseError{Pos: toks[k-1].pos, Msg: "dangling operator before 'dive'"}
				}
				cEnd = toks[k-1].pos
			}

			if toks[k+1].typ == tEOF {
				// trailing dive would silently disable element rules and filters; fail fast.
				return "", "", false, &ParseError{Pos: tk.pos, Msg: "'dive' must be followed by element rules"}
			}
			if toks[k+1].typ != tAnd {
				return "", "", false, &ParseError{Pos: toks[k+1].pos, Msg: "'dive' must be separated by &&"}
			}
			if k+2 >= len(toks) || !startsValue(toks[k+2].typ) {
				return "", "", false, &ParseError{Pos: toks[k+1].pos, Msg: "dangling operator after 'dive'"}
			}
			eStart := toks[k+1].pos + 2

			return strings.TrimSpace(input[:cEnd]), strings.TrimSpace(input[eStart:]), true, nil
		}
	}
	return input, "", false, nil
}

// ContainsDiveToken reports whether a standalone `dive` leaf appears at any depth.
func ContainsDiveToken(input string, isRawArg IsRawArgFunc) bool {
	toks, err := lex(input, isRawArg)
	if err != nil {
		return false
	}
	for k := range toks {
		if toks[k].typ == tLeaf && toks[k].name == "dive" && len(toks[k].args) == 0 {
			return true
		}
	}
	return false
}

// RemoveTopLevelLeaves drops top-level bare-leaf segments whose name (or trimmed
// source) is in names, re-joining survivors with " && ". Lexer-driven so a
// separator inside a quoted/raw arg isn't mistaken for structural.
func RemoveTopLevelLeaves(input string, names map[string]bool, isRawArg IsRawArgFunc) string {
	toks, err := lex(input, isRawArg)
	if err != nil {
		return input
	}
	var kept []string
	depth := 0
	segStart := 0
	var segToks []token

	flush := func(end int) {
		src := strings.TrimSpace(input[segStart:end])
		if src == "" {
			return
		}
		// 'dive' is reserved, never removed by name.
		if len(segToks) == 1 && segToks[0].typ == tLeaf && segToks[0].name != "dive" {
			if names[segToks[0].name] || names[src] {
				return
			}
		}
		kept = append(kept, src)
	}

	for k := range toks {
		t := toks[k]
		switch t.typ {
		case tAnd:
			if depth == 0 {
				flush(t.pos)
				segStart = t.pos + 2
				segToks = nil
				continue
			}
			segToks = append(segToks, t)
		case tEOF:
			flush(t.pos)
		case tLParen:
			depth++
			segToks = append(segToks, t)
		case tRParen:
			if depth > 0 {
				depth--
			}
			segToks = append(segToks, t)
		default:
			segToks = append(segToks, t)
		}
	}
	return strings.Join(kept, " && ")
}

func endsValue(t tokenType) bool { return t == tLeaf || t == tRParen }

func startsValue(t tokenType) bool { return t == tLeaf || t == tLParen || t == tNot }
