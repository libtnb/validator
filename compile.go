package validator

import (
	"strconv"

	"github.com/libtnb/validator/internal/dsl"
)

// resolver looks up a rule by signature; errRule wins when non-nil, ok=false fails compile.
type resolver func(signature string) (rule Rule, errRule ErrorRule, ok bool)

// compiled program: Diag appends leaf failures into dst exhaustively; Fast short-circuits.
type compiled struct {
	Fast func(f Field) bool
	Diag func(f Field, dst []FieldError) (bool, []FieldError)
}

type compiledNode struct {
	fast func(Field) bool
	diag func(Field, []FieldError) (bool, []FieldError)
}

// argChecker lets a rule validate its static args at compile time (e.g. a bad regex pattern).
type argChecker interface {
	CheckArgs(args []string) error
}

// leafCompiler is an optional hook letting a built-in rule return a pass function
// that skips per-eval bindArgs and interface dispatch. ErrorRule precedence is unaffected.
type leafCompiler interface {
	compilePasses(args []string) func(Field) bool
}

type argsBinder interface {
	WithArgs(args []string) Field
}

type argsField struct {
	Field
	args []string
}

func (a argsField) Attrs() []string { return a.args }

// compile turns an AST into a compiled program; unknown rules yield a *dsl.ParseError.
func compile(node dsl.Node, resolve resolver) (*compiled, error) {
	cn, err := compileNode(node, resolve, hasNumericAssertion(node))
	if err != nil {
		return nil, err
	}
	return &compiled{Fast: cn.fast, Diag: cn.diag}, nil
}

func compileNode(node dsl.Node, resolve resolver, numericHint bool) (compiledNode, error) {
	switch nd := node.(type) {
	case dsl.Leaf:
		return compileLeaf(nd, resolve, numericHint)

	case dsl.Not:
		child, err := compileNode(nd.X, resolve, numericHint)
		if err != nil {
			return compiledNode{}, err
		}
		return compiledNode{
			fast: func(f Field) bool { return !child.fast(f) },
			diag: func(f Field, dst []FieldError) (bool, []FieldError) {
				if !child.fast(f) {
					return true, dst
				}
				return false, appendGrown(dst, FieldError{
					Field:   f.Name(),
					Rule:    "!",
					Message: "The {field} is invalid.",
				})
			},
		}, nil

	case dsl.And:

		ops := flattenAnd(nd)
		children := make([]compiledNode, len(ops))
		for i, op := range ops {
			c, err := compileNode(op, resolve, numericHint)
			if err != nil {
				return compiledNode{}, err
			}
			children[i] = c
		}
		return compiledNode{
			fast: func(f Field) bool {
				for i := range children {
					if !children[i].fast(f) {
						return false
					}
				}
				return true
			},
			diag: func(f Field, dst []FieldError) (bool, []FieldError) {
				// Every operand runs, so ErrorRules must be idempotent.
				ok := true
				for i := range children {
					var okI bool
					okI, dst = children[i].diag(f, dst)
					ok = ok && okI
				}
				return ok, dst
			},
		}, nil

	case dsl.Or:

		ops := flattenOr(nd)
		children := make([]compiledNode, len(ops))
		for i, op := range ops {
			c, err := compileNode(op, resolve, numericHint)
			if err != nil {
				return compiledNode{}, err
			}
			children[i] = c
		}
		return compiledNode{
			fast: func(f Field) bool {
				for i := range children {
					if children[i].fast(f) {
						return true
					}
				}
				return false
			},
			diag: func(f Field, dst []FieldError) (bool, []FieldError) {
				// Reserve parent message; roll back to discard it if any branch passes.
				start := len(dst)
				dst = appendGrown(dst, FieldError{
					Field:   f.Name(),
					Rule:    "||",
					Message: "The {field} does not satisfy any of the required conditions.",
				})
				for i := range children {
					ok, d := children[i].diag(f, dst)
					if ok {
						return true, dst[:start]
					}
					dst = d
				}
				return false, dst
			},
		}, nil

	default:
		return compiledNode{}, &dsl.ParseError{Msg: "unknown ast node"}
	}
}

func compileLeaf(leaf dsl.Leaf, resolve resolver, numericHint bool) (compiledNode, error) {
	rule, errRule, ok := resolve(leaf.Name)
	if !ok || (rule == nil && errRule == nil) {
		return compiledNode{}, &dsl.ParseError{Pos: leaf.Pos, Msg: "unknown rule " + strconv.Quote(leaf.Name)}
	}
	name := leaf.Name
	args := leaf.Args

	var checked any = rule
	if errRule != nil {
		checked = errRule
	}
	if c, ok := checked.(argChecker); ok {
		if err := c.CheckArgs(args); err != nil {
			return compiledNode{}, &dsl.ParseError{Pos: leaf.Pos, Msg: "rule " + strconv.Quote(name) + ": " + err.Error()}
		}
	}

	// a numeric/number assertion in the expression flips size rules to
	// numeric-string comparison, compiled in once (zero eval-time cost)
	if numericHint && rule != nil {
		if nf, ok := rule.(numericHintForm); ok {
			rule = nf.withNumericHint()
		}
	}

	if errRule != nil {
		return compiledNode{
			fast: func(f Field) bool {
				return errRule.PassesE(bindArgs(f, args)) == nil
			},
			diag: func(f Field, dst []FieldError) (bool, []FieldError) {
				ff := bindArgs(f, args)
				if err := errRule.PassesE(ff); err != nil {
					// err.Error() is the message template; fall back to rule's if empty.
					msg := err.Error()
					if msg == "" {
						msg = errRule.Message()
					}
					return false, appendGrown(dst, FieldError{Field: f.Name(), Rule: name, Message: msg, Params: args})
				}
				return true, dst
			},
		}, nil
	}

	if lc, ok := rule.(leafCompiler); ok {
		if passes := lc.compilePasses(args); passes != nil {
			return compiledNode{
				fast: passes,
				diag: func(f Field, dst []FieldError) (bool, []FieldError) {
					if passes(f) {
						return true, dst
					}
					return false, appendGrown(dst, FieldError{
						Field:   f.Name(),
						Rule:    name,
						Message: rule.Message(),
						Params:  args,
					})
				},
			}, nil
		}
	}

	return compiledNode{
		fast: func(f Field) bool {
			return rule.Passes(bindArgs(f, args))
		},
		diag: func(f Field, dst []FieldError) (bool, []FieldError) {
			ff := bindArgs(f, args)
			if rule.Passes(ff) {
				return true, dst
			}
			return false, appendGrown(dst, FieldError{
				Field:   f.Name(),
				Rule:    name,
				Message: rule.Message(),
				Params:  args,
			})
		},
	}, nil
}

// hasNumericAssertion reports a numeric/number leaf on the top-level AND spine
// (not under || or !); such a marker flips size rules from rune length to numeric value.
func hasNumericAssertion(node dsl.Node) bool {
	switch nd := node.(type) {
	case dsl.Leaf:
		return nd.Name == "numeric" || nd.Name == "number"
	case dsl.And:
		return hasNumericAssertion(nd.L) || hasNumericAssertion(nd.R)
	}
	return false
}

func flattenOr(n dsl.Or) []dsl.Node {
	var out []dsl.Node
	var walk func(dsl.Node)
	walk = func(nd dsl.Node) {
		if o, ok := nd.(dsl.Or); ok {
			walk(o.L)
			walk(o.R)
			return
		}
		out = append(out, nd)
	}
	walk(n.L)
	walk(n.R)
	return out
}

func flattenAnd(n dsl.And) []dsl.Node {
	var out []dsl.Node
	var walk func(dsl.Node)
	walk = func(nd dsl.Node) {
		if a, ok := nd.(dsl.And); ok {
			walk(a.L)
			walk(a.R)
			return
		}
		out = append(out, nd)
	}
	walk(n.L)
	walk(n.R)
	return out
}

func bindArgs(f Field, args []string) Field {
	if b, ok := f.(argsBinder); ok {
		return b.WithArgs(args)
	}
	return argsField{Field: f, args: args}
}
