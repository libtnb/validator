package validator

import (
	"errors"
	"reflect"
	"testing"

	"github.com/libtnb/validator/internal/dsl"
)

type passRule struct {
	sig  string
	pass bool
}

func (r passRule) Signature() string { return r.sig }
func (r passRule) Passes(Field) bool { return r.pass }
func (r passRule) Message() string   { return "The {field} failed " + r.sig + "." }

type hasRule struct{}

func (hasRule) Signature() string { return "has" }
func (hasRule) Passes(f Field) bool {
	a := f.Attrs()
	return len(a) > 0 && a[0] == "yes"
}
func (hasRule) Message() string { return "The {field} needs yes." }

// okRule is an ErrorRule that passes only for "ok".
type okRule struct{}

func (okRule) Signature() string { return "ekey" }
func (okRule) PassesE(f Field) error {
	if f.Val().IsValid() && f.Val().Interface() == "ok" {
		return nil
	}
	return errors.New("The {field} must be ok.")
}
func (okRule) Message() string { return "The {field} ekey failed." }

func testResolver() resolver {
	return func(sig string) (Rule, ErrorRule, bool) {
		switch sig {
		case "t":
			return passRule{sig: "t", pass: true}, nil, true
		case "f":
			return passRule{sig: "f", pass: false}, nil, true
		case "has":
			return hasRule{}, nil, true
		case "ekey":
			return nil, okRule{}, true
		default:
			return nil, nil, false
		}
	}
}

func mustCompile(t *testing.T, expr string) *compiled {
	t.Helper()
	node, err := dsl.Parse(expr, nil)
	if err != nil {
		t.Fatalf("dsl.Parse(%q): %v", expr, err)
	}
	c, err := compile(node, testResolver())
	if err != nil {
		t.Fatalf("compile(%q): %v", expr, err)
	}
	return c
}

func TestCompileFastLogic(t *testing.T) {
	cases := []struct {
		expr string
		want bool
	}{
		{"t", true},
		{"f", false},
		{"t && t", true},
		{"t && f", false},
		{"f || t", true},
		{"f || f", false},
		{"!f", true},
		{"!t", false},
		{"f && t || t", true}, // (f&&t)||t
		{"t || f && f", true}, // t||(f&&f)
		{"!f && t", true},     // (!f)&&t
		{"!(f || f)", true},
	}
	f := fakeField{name: "x"}
	for _, c := range cases {
		if got := mustCompile(t, c.expr).Fast(f); got != c.want {
			t.Errorf("Fast(%q) = %v want %v", c.expr, got, c.want)
		}
	}
}

func TestCompileArgBinding(t *testing.T) {
	f := fakeField{name: "x"}
	if !mustCompile(t, "has:yes").Fast(f) {
		t.Error(`has:yes should pass`)
	}
	if mustCompile(t, "has:no").Fast(f) {
		t.Error(`has:no should fail`)
	}
}

func TestCompileErrorRule(t *testing.T) {
	if !mustCompile(t, "ekey").Fast(fakeField{name: "x", val: reflect.ValueOf("ok")}) {
		t.Error("ekey should pass for ok")
	}
	ok, errs := mustCompile(t, "ekey").Diag(fakeField{name: "x", val: reflect.ValueOf("bad")}, nil)
	// dsl emits the raw template; root resolves placeholders.
	if ok || len(errs) != 1 || errs[0].Message != "The {field} must be ok." {
		t.Errorf("ekey diag = %v %v", ok, errs)
	}
}

func TestCompileDiagAggregation(t *testing.T) {
	f := fakeField{name: "email"}

	ok, errs := mustCompile(t, "f && f").Diag(f, nil)
	if ok || len(errs) != 2 {
		t.Errorf("'f && f' diag = %v, %d errs want false,2", ok, len(errs))
	}

	// OR, both failing: synthesized parent message plus each branch.
	ok, errs = mustCompile(t, "f || f").Diag(f, nil)
	if ok || len(errs) != 3 {
		t.Errorf("'f || f' diag = %v, %d errs want false,3", ok, len(errs))
	}
	if errs[0].Rule != "||" {
		t.Errorf("first OR error should be the synthesized parent, got %q", errs[0].Rule)
	}

	// success path allocates no errors: nil slice, not empty.
	if ok, errs := mustCompile(t, "t && t").Diag(f, nil); !ok || errs != nil {
		t.Errorf("'t && t' diag = %v, %v want true,nil", ok, errs)
	}

	_, errs = mustCompile(t, "f").Diag(f, nil)
	if len(errs) != 1 || errs[0].Message != "The {field} failed f." {
		t.Errorf("raw template = %q", errs[0].Message)
	}
}

func TestCompileFailFastUnknownRule(t *testing.T) {
	node, err := dsl.Parse("t && nope", nil)
	if err != nil {
		t.Fatalf("dsl.Parse: %v", err)
	}
	if _, err := compile(node, testResolver()); err == nil {
		t.Error("compile should fail fast on unknown rule")
	} else if _, ok := err.(*dsl.ParseError); !ok {
		t.Errorf("error type = %T want *dsl.ParseError", err)
	}
}

type countRule struct{ calls *int }

func (countRule) Signature() string   { return "cf" }
func (r countRule) Passes(Field) bool { *r.calls++; return false }
func (countRule) Message() string     { return "The {field} cf failed." }

// TestOrDiagEvaluatesEachBranchOnce: diag OR evaluates each branch once (ErrorRules may hit a DB).
func TestOrDiagEvaluatesEachBranchOnce(t *testing.T) {
	var calls int
	res := func(sig string) (Rule, ErrorRule, bool) {
		if sig == "cf" {
			return countRule{calls: &calls}, nil, true
		}
		return nil, nil, false
	}
	node, err := dsl.Parse("cf || cf", nil)
	if err != nil {
		t.Fatal(err)
	}
	c, err := compile(node, res)
	if err != nil {
		t.Fatal(err)
	}
	calls = 0
	c.Diag(fakeField{name: "x"}, nil)
	if calls != 2 {
		t.Errorf("each OR branch should be evaluated exactly once (2 total), got %d", calls)
	}
}

// TestAndFastShortCircuitsDiagExhaustive: && Fast short-circuits, Diag is exhaustive so Errors() reports every failure.
func TestAndFastShortCircuitsDiagExhaustive(t *testing.T) {
	var calls int
	res := func(sig string) (Rule, ErrorRule, bool) {
		if sig == "cf" {
			return countRule{calls: &calls}, nil, true
		}
		return nil, nil, false
	}
	node, err := dsl.Parse("cf && cf", nil)
	if err != nil {
		t.Fatal(err)
	}
	c, err := compile(node, res)
	if err != nil {
		t.Fatal(err)
	}

	calls = 0
	c.Fast(fakeField{name: "x"})
	if calls != 1 {
		t.Errorf("Fast '&&' should short-circuit after first false (1 call), got %d", calls)
	}
	calls = 0
	c.Diag(fakeField{name: "x"}, nil)
	if calls != 2 {
		t.Errorf("Diag '&&' should evaluate both sides once (2 calls), got %d", calls)
	}
}

// TestFastWrapperPathAllocs: a Field lacking WithArgs costs at most one argsField wrapper alloc per leaf.
func TestFastWrapperPathAllocs(t *testing.T) {
	c := mustCompile(t, "has:yes")
	var ext Field = fakeField{name: "x"} // no WithArgs -> wrapper path
	allocs := testing.AllocsPerRun(100, func() { _ = c.Fast(ext) })
	if allocs > 1 {
		t.Errorf("wrapper path allocated %v per leaf, want <=1 (regression means extra boxing)", allocs)
	}
}
