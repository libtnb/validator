package validator

import (
	"errors"
	"reflect"
	"slices"
	"time"
)

func init() {
	registerRules(
		&requiredIfRule{},
		&requiredUnlessRule{},
		&requiredWithRule{},
		&requiredWithoutRule{},
		&requiredWithAllRule{},
		&requiredWithoutAllRule{},
		&excludedIfRule{},
		&excludedUnlessRule{},
		&excludedWithRule{},
		&excludedWithoutRule{},
		&sameRule{},
		&differentRule{},
		&eqFieldRule{},
		&neFieldRule{},
		&gtFieldRule{},
		&gteFieldRule{},
		&ltFieldRule{},
		&lteFieldRule{},
		&confirmedRule{},
	)
}

var (
	_ Rule = (*requiredIfRule)(nil)
	_ Rule = (*requiredUnlessRule)(nil)
	_ Rule = (*requiredWithRule)(nil)
	_ Rule = (*requiredWithoutRule)(nil)
	_ Rule = (*requiredWithAllRule)(nil)
	_ Rule = (*requiredWithoutAllRule)(nil)
	_ Rule = (*excludedIfRule)(nil)
	_ Rule = (*excludedUnlessRule)(nil)
	_ Rule = (*excludedWithRule)(nil)
	_ Rule = (*excludedWithoutRule)(nil)
	_ Rule = (*sameRule)(nil)
	_ Rule = (*differentRule)(nil)
	_ Rule = (*eqFieldRule)(nil)
	_ Rule = (*neFieldRule)(nil)
	_ Rule = (*gtFieldRule)(nil)
	_ Rule = (*gteFieldRule)(nil)
	_ Rule = (*ltFieldRule)(nil)
	_ Rule = (*lteFieldRule)(nil)
	_ Rule = (*confirmedRule)(nil)

	// Missing args make these conditional rules pass vacuously (fail-open), so
	// arity is checked at compile time; the value-comparison family fails closed
	// at eval time and needs no check.
	_ argChecker = (*requiredIfRule)(nil)
	_ argChecker = (*requiredUnlessRule)(nil)
	_ argChecker = (*requiredWithRule)(nil)
	_ argChecker = (*requiredWithoutRule)(nil)
	_ argChecker = (*requiredWithAllRule)(nil)
	_ argChecker = (*requiredWithoutAllRule)(nil)
	_ argChecker = (*excludedIfRule)(nil)
	_ argChecker = (*excludedUnlessRule)(nil)
	_ argChecker = (*excludedWithRule)(nil)
	_ argChecker = (*excludedWithoutRule)(nil)
)

// checkSiblingValueArgs: rule:Field,val… needs a sibling name plus at least one value.
func checkSiblingValueArgs(args []string) error {
	if len(args) < 2 {
		return errors.New("needs a sibling field and at least one value (rule:Field,val…)")
	}
	return nil
}

// checkSiblingArgs: rule:Field… needs at least one sibling name.
func checkSiblingArgs(args []string) error {
	if len(args) == 0 {
		return errors.New("needs at least one sibling field name")
	}
	return nil
}

// requiredIfRule: required when another field equals any listed value.
type requiredIfRule struct{ strict bool }

func (r *requiredIfRule) Signature() string { return "required_if" }

func (r *requiredIfRule) Passes(f Field) bool {
	attrs := f.Attrs()
	if len(attrs) < 2 {
		return true
	}
	other, ok := crossSibling(f, attrs[0])
	if !ok {
		return true
	}
	if slices.Contains(attrs[1:], cmpString(other)) {
		return present(f, r.strict)
	}
	return true
}

func (r *requiredIfRule) CheckArgs(args []string) error { return checkSiblingValueArgs(args) }

func (r *requiredIfRule) Message() string {
	return "The {field} field is required when {0} is {1+}."
}

func (r *requiredIfRule) withStrict() Rule { return &requiredIfRule{strict: true} }

// requiredUnlessRule: required unless another field equals any listed value.
type requiredUnlessRule struct{ strict bool }

func (r *requiredUnlessRule) Signature() string { return "required_unless" }

func (r *requiredUnlessRule) Passes(f Field) bool {
	attrs := f.Attrs()
	if len(attrs) < 2 {
		return true
	}
	other, ok := crossSibling(f, attrs[0])
	// absent sibling can't match -> required
	if !ok {
		return present(f, r.strict)
	}
	if slices.Contains(attrs[1:], cmpString(other)) {
		return true
	}
	return present(f, r.strict)
}

func (r *requiredUnlessRule) CheckArgs(args []string) error { return checkSiblingValueArgs(args) }

func (r *requiredUnlessRule) Message() string {
	return "The {field} field is required unless {0} is {1+}."
}

func (r *requiredUnlessRule) withStrict() Rule { return &requiredUnlessRule{strict: true} }

// requiredWithRule: required when any listed field is present.
type requiredWithRule struct{ strict bool }

func (r *requiredWithRule) Signature() string { return "required_with" }

func (r *requiredWithRule) Passes(f Field) bool {
	attrs := f.Attrs()
	if len(attrs) == 0 {
		return true
	}
	for _, name := range attrs {
		if v, ok := crossSibling(f, name); ok && !IsEmptyValue(v) {
			return present(f, r.strict)
		}
	}
	return true
}

func (r *requiredWithRule) CheckArgs(args []string) error { return checkSiblingArgs(args) }

func (r *requiredWithRule) Message() string {
	return "The {field} field is required when {0} is present."
}

func (r *requiredWithRule) withStrict() Rule { return &requiredWithRule{strict: true} }

// requiredWithoutRule: required when any listed field is missing/empty.
type requiredWithoutRule struct{ strict bool }

func (r *requiredWithoutRule) Signature() string { return "required_without" }

func (r *requiredWithoutRule) Passes(f Field) bool {
	attrs := f.Attrs()
	if len(attrs) == 0 {
		return true
	}
	for _, name := range attrs {
		v, ok := crossSibling(f, name)
		if !ok || IsEmptyValue(v) {
			return present(f, r.strict)
		}
	}
	return true
}

func (r *requiredWithoutRule) CheckArgs(args []string) error { return checkSiblingArgs(args) }

func (r *requiredWithoutRule) Message() string {
	return "The {field} field is required when {0} is not present."
}

func (r *requiredWithoutRule) withStrict() Rule { return &requiredWithoutRule{strict: true} }

// requiredWithAllRule: required when every listed field is present.
type requiredWithAllRule struct{ strict bool }

func (r *requiredWithAllRule) Signature() string { return "required_with_all" }

func (r *requiredWithAllRule) Passes(f Field) bool {
	attrs := f.Attrs()
	if len(attrs) == 0 {
		return true
	}
	for _, name := range attrs {
		if v, ok := crossSibling(f, name); !ok || IsEmptyValue(v) {
			return true
		}
	}
	return present(f, r.strict)
}

func (r *requiredWithAllRule) CheckArgs(args []string) error { return checkSiblingArgs(args) }

func (r *requiredWithAllRule) Message() string {
	return "The {field} field is required when {0+} are present."
}

func (r *requiredWithAllRule) withStrict() Rule { return &requiredWithAllRule{strict: true} }

// requiredWithoutAllRule: required when every listed field is missing/empty.
type requiredWithoutAllRule struct{ strict bool }

func (r *requiredWithoutAllRule) Signature() string { return "required_without_all" }

func (r *requiredWithoutAllRule) Passes(f Field) bool {
	attrs := f.Attrs()
	if len(attrs) == 0 {
		return true
	}
	for _, name := range attrs {
		if v, ok := crossSibling(f, name); ok && !IsEmptyValue(v) {
			return true
		}
	}
	return present(f, r.strict)
}

func (r *requiredWithoutAllRule) CheckArgs(args []string) error { return checkSiblingArgs(args) }

func (r *requiredWithoutAllRule) Message() string {
	return "The {field} field is required when none of {0+} are present."
}

func (r *requiredWithoutAllRule) withStrict() Rule { return &requiredWithoutAllRule{strict: true} }

// excludedIfRule: must be empty when another field equals any listed value.
type excludedIfRule struct{}

func (r *excludedIfRule) Signature() string { return "excluded_if" }

func (r *excludedIfRule) Passes(f Field) bool {
	attrs := f.Attrs()
	if len(attrs) < 2 || isEmptyV(f.Val()) {
		return true
	}
	other, ok := crossSibling(f, attrs[0])
	if !ok {
		return true
	}
	return !slices.Contains(attrs[1:], cmpString(other))
}

func (r *excludedIfRule) CheckArgs(args []string) error { return checkSiblingValueArgs(args) }

func (r *excludedIfRule) Message() string {
	return "The {field} field must be empty when {0} is {1+}."
}

// excludedUnlessRule: must be empty unless another field equals any listed value.
type excludedUnlessRule struct{}

func (r *excludedUnlessRule) Signature() string { return "excluded_unless" }

func (r *excludedUnlessRule) Passes(f Field) bool {
	attrs := f.Attrs()
	if len(attrs) < 2 || isEmptyV(f.Val()) {
		return true
	}
	// absent sibling can't match -> exclusion applies
	other, ok := crossSibling(f, attrs[0])
	if !ok {
		return false
	}
	return slices.Contains(attrs[1:], cmpString(other))
}

func (r *excludedUnlessRule) CheckArgs(args []string) error { return checkSiblingValueArgs(args) }

func (r *excludedUnlessRule) Message() string {
	return "The {field} field must be empty unless {0} is {1+}."
}

// excludedWithRule: must be empty when any listed field is present.
type excludedWithRule struct{}

func (r *excludedWithRule) Signature() string { return "excluded_with" }

func (r *excludedWithRule) Passes(f Field) bool {
	attrs := f.Attrs()
	if len(attrs) == 0 || isEmptyV(f.Val()) {
		return true
	}
	for _, name := range attrs {
		if v, ok := crossSibling(f, name); ok && !IsEmptyValue(v) {
			return false
		}
	}
	return true
}

func (r *excludedWithRule) CheckArgs(args []string) error { return checkSiblingArgs(args) }

func (r *excludedWithRule) Message() string {
	return "The {field} field must be empty when {0} is present."
}

// excludedWithoutRule: must be empty when any listed field is missing/empty.
type excludedWithoutRule struct{}

func (r *excludedWithoutRule) Signature() string { return "excluded_without" }

func (r *excludedWithoutRule) Passes(f Field) bool {
	attrs := f.Attrs()
	if len(attrs) == 0 || isEmptyV(f.Val()) {
		return true
	}
	for _, name := range attrs {
		if v, ok := crossSibling(f, name); !ok || IsEmptyValue(v) {
			return false
		}
	}
	return true
}

func (r *excludedWithoutRule) CheckArgs(args []string) error { return checkSiblingArgs(args) }

func (r *excludedWithoutRule) Message() string {
	return "The {field} field must be empty when {0} is not present."
}

// sameRule: equals another field string-wise.
type sameRule struct{}

func (r *sameRule) Signature() string { return "same" }

func (r *sameRule) Passes(f Field) bool {
	return crossStringCompare(f, true, false)
}

func (r *sameRule) Message() string { return "The {field} and {0} must match." }

// differentRule: differs from another field string-wise.
type differentRule struct{}

func (r *differentRule) Signature() string { return "different" }

func (r *differentRule) Passes(f Field) bool {
	return crossStringCompare(f, false, true)
}

func (r *differentRule) Message() string { return "The {field} and {0} must be different." }

// eqFieldRule: alias of same.
type eqFieldRule struct{}

func (r *eqFieldRule) Signature() string { return "eqfield" }

func (r *eqFieldRule) Passes(f Field) bool {
	return crossStringCompare(f, true, false)
}

func (r *eqFieldRule) Message() string { return "The {field} and {0} must match." }

// neFieldRule: alias of different.
type neFieldRule struct{}

func (r *neFieldRule) Signature() string { return "nefield" }

func (r *neFieldRule) Passes(f Field) bool {
	return crossStringCompare(f, false, true)
}

func (r *neFieldRule) Message() string { return "The {field} and {0} must be different." }

// gtFieldRule: numerically greater than another field.
type gtFieldRule struct{}

func (r *gtFieldRule) Signature() string { return "gtfield" }

func (r *gtFieldRule) Passes(f Field) bool {
	return crossNumericCompare(f, func(self, other float64) bool { return self > other })
}

func (r *gtFieldRule) Message() string { return "The {field} must be greater than {0}." }

// gteFieldRule: numerically >= another field.
type gteFieldRule struct{}

func (r *gteFieldRule) Signature() string { return "gtefield" }

func (r *gteFieldRule) Passes(f Field) bool {
	return crossNumericCompare(f, func(self, other float64) bool { return self >= other })
}

func (r *gteFieldRule) Message() string { return "The {field} must be greater than or equal to {0}." }

// ltFieldRule: numerically less than another field.
type ltFieldRule struct{}

func (r *ltFieldRule) Signature() string { return "ltfield" }

func (r *ltFieldRule) Passes(f Field) bool {
	return crossNumericCompare(f, func(self, other float64) bool { return self < other })
}

func (r *ltFieldRule) Message() string { return "The {field} must be less than {0}." }

// lteFieldRule: numerically <= another field.
type lteFieldRule struct{}

func (r *lteFieldRule) Signature() string { return "ltefield" }

func (r *lteFieldRule) Passes(f Field) bool {
	return crossNumericCompare(f, func(self, other float64) bool { return self <= other })
}

func (r *lteFieldRule) Message() string { return "The {field} must be less than or equal to {0}." }

// confirmedRule: "<field>_confirmation" exists and equals the field string-wise.
type confirmedRule struct{}

func (r *confirmedRule) Signature() string { return "confirmed" }

func (r *confirmedRule) Passes(f Field) bool {
	if isEmptyV(f.Val()) {
		return true
	}
	// stable identity, not display name: a dive element's bracketed key ("pw[0]") must not shape the lookup.
	base := f.Name()
	if sn, ok := f.(interface{ scopeName() string }); ok {
		base = sn.scopeName()
	}
	other, ok := crossSibling(f, base+"_confirmation")
	if !ok {
		return false
	}
	return cmpString(f.Val()) == cmpString(other)
}

func (r *confirmedRule) Message() string { return "The {field} confirmation does not match." }

// siblingValuer is the engine's value-only sibling lookup (implemented by *field).
type siblingValuer interface {
	SiblingValue(name string) (reflect.Value, bool)
}

func crossSibling(f Field, name string) (reflect.Value, bool) {
	if sv, ok := f.(siblingValuer); ok {
		return sv.SiblingValue(name)
	}
	if s, ok := f.Sibling(name); ok {
		return s.Val(), true
	}
	return reflect.Value{}, false
}

// crossStringCompare: empty self passes (omitempty); missing arg fails closed.
func crossStringCompare(f Field, equal, whenMissing bool) bool {
	if isEmptyV(f.Val()) {
		return true
	}
	attrs := f.Attrs()
	if len(attrs) == 0 {
		return false
	}
	other, ok := crossSibling(f, attrs[0])
	if !ok {
		return whenMissing
	}
	eq := cmpString(f.Val()) == cmpString(other)
	return eq == equal
}

// crossNumericCompare: empty self passes (omitempty); a time.Time pair compares
// chronologically (its Compare result feeds cmp as ±1/0 vs 0); otherwise
// not-both-numeric fails closed.
func crossNumericCompare(f Field, cmp func(self, other float64) bool) bool {
	rv := f.Val()
	if isEmptyV(rv) {
		return true
	}
	attrs := f.Attrs()
	if len(attrs) == 0 {
		return false
	}
	ov, found := crossSibling(f, attrs[0])
	if !found {
		return false
	}
	if rv.Type() == timeType && ov.IsValid() && ov.Type() == timeType {
		// a zero-time sibling is an empty reference: fail closed, like a blank numeric sibling
		o := ov.Interface().(time.Time)
		if o.IsZero() {
			return false
		}
		return cmp(float64(rv.Interface().(time.Time).Compare(o)), 0)
	}
	a, aok := numericValue(rv)
	b, bok := numericValue(ov)
	if !aok || !bok {
		return false
	}
	return cmp(a, b)
}
