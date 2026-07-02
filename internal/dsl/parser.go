package dsl

// Bound recursion depth and node count so pathological input can't overflow the stack.
const maxNestingDepth = 10000
const maxNodes = 50000

// Parse builds an AST from a rule expression. Precedence: ! > && > ||, left-associative.
func Parse(input string, isRawArg IsRawArgFunc) (Node, error) {
	toks, err := lex(input, isRawArg)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks}
	node, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.cur().typ != tEOF {
		return nil, &ParseError{Pos: p.cur().pos, Msg: "unexpected token after expression"}
	}
	return node, nil
}

type parser struct {
	toks  []token
	pos   int
	depth int
	nodes int
}

func (p *parser) cur() token { return p.toks[p.pos] }

func (p *parser) count() error {
	p.nodes++
	if p.nodes > maxNodes {
		return &ParseError{Pos: p.cur().pos, Msg: "expression too large"}
	}
	return nil
}

func (p *parser) enter() error {
	p.depth++
	if p.depth > maxNestingDepth {
		return &ParseError{Pos: p.cur().pos, Msg: "expression nesting too deep"}
	}
	return nil
}

func (p *parser) leave() { p.depth-- }

func (p *parser) advance() token {
	t := p.toks[p.pos]
	if p.pos < len(p.toks)-1 {
		p.pos++
	}
	return t
}

func (p *parser) parseOr() (Node, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.cur().typ == tOr {
		p.advance()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		if err := p.count(); err != nil {
			return nil, err
		}
		left = Or{L: left, R: right}
	}
	return left, nil
}

func (p *parser) parseAnd() (Node, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for p.cur().typ == tAnd {
		p.advance()
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		if err := p.count(); err != nil {
			return nil, err
		}
		left = And{L: left, R: right}
	}
	return left, nil
}

func (p *parser) parseUnary() (Node, error) {
	if p.cur().typ == tNot {
		p.advance()
		if err := p.enter(); err != nil {
			return nil, err
		}
		x, err := p.parseUnary()
		p.leave()
		if err != nil {
			return nil, err
		}
		return Not{X: x}, nil
	}
	return p.parsePrimary()
}

func (p *parser) parsePrimary() (Node, error) {
	t := p.cur()
	switch t.typ {
	case tLeaf:
		p.advance()
		return Leaf{Name: t.name, Args: t.args, Pos: t.pos}, nil
	case tLParen:
		p.advance()
		if err := p.enter(); err != nil {
			return nil, err
		}
		node, err := p.parseOr()
		p.leave()
		if err != nil {
			return nil, err
		}
		if p.cur().typ != tRParen {
			return nil, &ParseError{Pos: p.cur().pos, Msg: "expected ')'"}
		}
		p.advance()
		return node, nil
	case tEOF:
		return nil, &ParseError{Pos: t.pos, Msg: "unexpected end of expression"}
	default:
		return nil, &ParseError{Pos: t.pos, Msg: "expected a rule or '('"}
	}
}
