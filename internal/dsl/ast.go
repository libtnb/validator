// Package dsl is the boolean rule-expression engine: lexer, parser (! > && > ||), AST, dive splitting.
package dsl

// Node is a node in a parsed rule expression.
type Node interface {
	node()
}

// Leaf is a single rule invocation, e.g. in:a,b,c.
type Leaf struct {
	Name string
	Args []string
	Pos  int
}

func (Leaf) node() {}

// Not is logical negation: !X.
type Not struct {
	X Node
}

func (Not) node() {}

// And is logical conjunction: L && R.
type And struct {
	L, R Node
}

func (And) node() {}

// Or is logical disjunction: L || R.
type Or struct {
	L, R Node
}

func (Or) node() {}
