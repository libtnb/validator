package validator

import (
	"context"
	"fmt"
	"testing"
)

func TestConcurrentValidation(t *testing.T) {
	// threshold 1 forces the parallel path; run -race.
	v := MustNew(WithParallel(1))

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
	vd := v.MustMap(data, sigs)
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

// TestConcurrentMatchesSerial: parallel and serial must produce identical errors.
func TestConcurrentMatchesSerial(t *testing.T) {
	sigs := map[string]string{}
	data := map[string]any{}
	for i := range 20 {
		f := fmt.Sprintf("f%02d", i)
		sigs[f] = "required && email"
		data[f] = "not-an-email"
	}

	serial := MustNew().MustMap(data, sigs)
	serial.Validate(context.Background())

	parallel := MustNew(WithParallel(1)).MustMap(data, sigs)
	parallel.Validate(context.Background())

	if serial.Errors().String() != parallel.Errors().String() {
		t.Errorf("serial and parallel output differ:\n serial=%q\n parallel=%q", serial.Errors().String(), parallel.Errors().String())
	}
}
