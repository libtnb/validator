package validator

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

func TestConcurrentValidation(t *testing.T) {
	// threshold 1 forces the parallel path; run -race.
	v := NewValidator(WithParallel(1))

	sigs := map[string]string{}
	data := map[string]any{}
	for i := range 50 {
		f := fmt.Sprintf("f%02d", i)
		sigs[f] = "required && numeric"
		if i%2 == 0 {
			data[f] = i + 1 // non-zero: 0 is "empty" and fails required
		} else {
			data[f] = "bad"
		}
	}
	vd := v.Map(data, sigs)
	vd.Validate(context.Background())

	for i := range 50 {
		f := fmt.Sprintf("f%02d", i)
		if i%2 == 0 && vd.Errors().Has(f) {
			t.Errorf("%s (even) should pass", f)
		}
		if i%2 == 1 && !vd.Errors().Has(f) {
			t.Errorf("%s (odd) should fail", f)
		}
	}
}

// A Register* racing a type's first plan build must never durably publish a
// plan bound to the old registry: validations created after Register* returns
// always see the new rule.
func TestRegisterRaceNoStalePlan(t *testing.T) {
	type T struct {
		A string `validate:"flip"`
	}
	for round := range 60 {
		v := NewValidator()
		v.RegisterFunc("flip", func(Field) bool { return true }, "m")
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); _ = v.Valid(T{A: "x"}) }() // races the first build
		go func() {
			defer wg.Done()
			v.RegisterFunc("flip", func(Field) bool { return false }, "m")
		}()
		wg.Wait()
		if v.Valid(T{A: "x"}) {
			t.Fatalf("round %d: a validation created after Register* saw a stale plan", round)
		}
	}
}

// A concurrent reader must never TRUST a struct plan a straddling builder
// published for a now-obsolete registry: the per-read gen check rejects it even
// in the transient window before the builder evicts its own stale entry.
func TestStructPlanNoTransientStaleRead(t *testing.T) {
	type T struct {
		A string `validate:"flip"`
	}
	for round := range 40 {
		v := NewValidator()
		v.RegisterFunc("flip", func(Field) bool { return true }, "m")
		var wg sync.WaitGroup
		// one registrar + many concurrent readers racing the first build
		wg.Go(func() {
			v.RegisterFunc("flip", func(Field) bool { return false }, "m")
		})
		for range 8 {
			wg.Go(func() { _ = v.Valid(T{A: "x"}); _ = v.Struct(T{A: "x"}).Fails() })
		}
		wg.Wait()
		// after every Register* has returned, the rule is definitively flip=false
		if v.Valid(T{A: "x"}) {
			t.Fatalf("round %d: a validation after Register* returned saw a stale struct plan", round)
		}
	}
}

// TestConcurrentMatchesSerial: parallel and serial must produce identical errors.
func TestConcurrentMatchesSerial(t *testing.T) {
	sigs := map[string]string{}
	data := map[string]any{}
	for i := range 20 {
		f := fmt.Sprintf("f%02d", i)
		sigs[f] = "required && email"
		data[f] = "not-an-email"
	}

	serial := NewValidator().Map(data, sigs)
	serial.Validate(context.Background())

	parallel := NewValidator(WithParallel(1)).Map(data, sigs)
	parallel.Validate(context.Background())

	if serial.Errors().String() != parallel.Errors().String() {
		t.Errorf("serial and parallel output differ:\n serial=%q\n parallel=%q", serial.Errors().String(), parallel.Errors().String())
	}
}
