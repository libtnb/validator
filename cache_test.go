package validator

import (
	"fmt"
	"testing"
)

// TestExprCacheBounded checks the compiled-expression cache stays bounded.
func TestExprCacheBounded(t *testing.T) {
	v := NewValidator(WithoutBuiltinRules())
	v.RegisterFunc("tag", func(Field) bool { return true }, "m")
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
	v := NewValidator(WithoutBuiltinRules())
	v.RegisterFunc("yes", func(Field) bool { return true }, "m")
	v.RegisterFunc("no", func(Field) bool { return false }, "m")

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

	if !c1.Fast(&field{name: "x"}) {
		t.Error("yes && yes should pass")
	}
	c3, _ := v.compile("yes && no")
	if c3.Fast(&field{name: "x"}) {
		t.Error("yes && no should fail")
	}

	// unknown rule must fail at compile time, not validation time
	if _, err := v.compile("ghost"); err == nil {
		t.Error("unknown rule should fail compilation")
	}
}

// TestExprCompileAllocFree guards that evaluating a cached program over the root *field is alloc-free.
func TestExprCompileAllocFree(t *testing.T) {
	v := NewValidator(WithoutBuiltinRules())
	v.RegisterFunc("yes", func(Field) bool { return true }, "m")
	c, err := v.compile("yes && yes")
	if err != nil {
		t.Fatal(err)
	}
	f := &field{name: "x"}
	allocs := testing.AllocsPerRun(100, func() {
		_ = c.Fast(f)
	})
	if allocs != 0 {
		t.Errorf("Fast eval allocated %v times, want 0", allocs)
	}
}

// TestExprCompileAllocFreeWithArgs guards alloc-free eval with rule args and multiple leaves.
func TestExprCompileAllocFreeWithArgs(t *testing.T) {
	v := NewValidator(WithoutBuiltinRules())
	v.RegisterFunc("has", func(f Field) bool { return len(f.Attrs()) > 0 }, "m")
	c, err := v.compile("has:a && has:b")
	if err != nil {
		t.Fatal(err)
	}
	f := &field{name: "x"}
	allocs := testing.AllocsPerRun(100, func() {
		_ = c.Fast(f)
	})
	if allocs != 0 {
		t.Errorf("parameterized multi-leaf Fast over *field allocated %v, want 0", allocs)
	}
}
