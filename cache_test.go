package validator

import (
	"fmt"
	"testing"
)

// TestExprCacheBounded checks the compiled-expression cache stays bounded.
func TestExprCacheBounded(t *testing.T) {
	v := MustNew(WithoutBuiltinRules(), WithRuleFunc("tag", func(*Field) bool { return true }, "m"))
	for i := 0; i < maxExprCache+50; i++ {
		if _, err := v.compile(fmt.Sprintf("tag:%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(*v.exprCache.m.Load()); got > maxExprCache {
		t.Errorf("exprCache grew to %d, exceeds bound %d", got, maxExprCache)
	}
}

func TestExprCache(t *testing.T) {
	v := MustNew(
		WithoutBuiltinRules(),
		WithRuleFunc("yes", func(*Field) bool { return true }, "m"),
		WithRuleFunc("no", func(*Field) bool { return false }, "m"),
	)

	c1, err := v.compile("yes && yes")
	if err != nil {
		t.Fatal(err)
	}
	c2, err := v.compile("yes && yes")
	if err != nil {
		t.Fatal(err)
	}
	if c1 != c2 {
		t.Error("the same expression must be compiled once and served from cache")
	}

	if passed, err := c1.Fast(&Field{name: "x"}); err != nil || !passed {
		t.Error("yes && yes should pass")
	}
	c3, _ := v.compile("yes && no")
	if passed, err := c3.Fast(&Field{name: "x"}); err != nil || passed {
		t.Error("yes && no should fail")
	}

	// unknown rule must fail at compile time, not validation time
	if _, err := v.compile("ghost"); err == nil {
		t.Error("unknown rule should fail compilation")
	}
}

// TestExprCompileAllocFree guards that evaluating a cached program over the root *Field is alloc-free.
func TestExprCompileAllocFree(t *testing.T) {
	v := MustNew(WithoutBuiltinRules(), WithRuleFunc("yes", func(*Field) bool { return true }, "m"))
	c, err := v.compile("yes && yes")
	if err != nil {
		t.Fatal(err)
	}
	f := &Field{name: "x"}
	allocs := testing.AllocsPerRun(100, func() {
		_, _ = c.Fast(f)
	})
	if allocs != 0 {
		t.Errorf("Fast eval allocated %v times, want 0", allocs)
	}
}

// TestExprCompileAllocFreeWithArgs guards alloc-free eval with rule args and multiple leaves.
func TestExprCompileAllocFreeWithArgs(t *testing.T) {
	v := MustNew(WithoutBuiltinRules(),
		WithRuleFunc("has", func(f *Field) bool { return len(f.Attrs()) > 0 }, "m"))
	c, err := v.compile("has:a && has:b")
	if err != nil {
		t.Fatal(err)
	}
	f := &Field{name: "x"}
	allocs := testing.AllocsPerRun(100, func() {
		_, _ = c.Fast(f)
	})
	if allocs != 0 {
		t.Errorf("parameterized multi-leaf Fast over *Field allocated %v, want 0", allocs)
	}
}
