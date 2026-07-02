package dsl

type tokenType int

const (
	tEOF tokenType = iota
	tAnd
	tOr
	tNot
	tLParen
	tRParen
	tLeaf
)

type token struct {
	typ  tokenType
	name string
	args []string
	pos  int
}
