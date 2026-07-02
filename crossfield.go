package validator

import (
	"reflect"
	"slices"
)

func init() {
	registerRules(
		&requiredIfRule{},
		&requiredUnlessRule{},
		&requiredWithRule{},
		&requiredWithoutRule{},
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
	_ Rule = (*sameRule)(nil)
	_ Rule = (*differentRule)(nil)
	_ Rule = (*eqFieldRule)(nil)
	_ Rule = (*neFieldRule)(nil)
	_ Rule = (*gtFieldRule)(nil)
	_ Rule = (*gteFieldRule)(nil)
	_ Rule = (*ltFieldRule)(nil)
	_ Rule = (*lteFieldRule)(nil)
	_ Rule = (*confirmedRule)(nil)
)

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

func (r *requiredWithoutRule) Message() string {
	return "The {field} field is required when {0} is not present."
}

func (r *requiredWithoutRule) withStrict() Rule { return &requiredWithoutRule{strict: true} }

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

// crossNumericCompare: empty self passes (omitempty); not-both-numeric fails closed.
func crossNumericCompare(f Field, cmp func(self, other float64) bool) bool {
	if isEmptyV(f.Val()) {
		return true
	}
	a, b, ok := crossNumeric(f)
	if !ok {
		return false
	}
	return cmp(a, b)
}

func crossNumeric(f Field) (self float64, other float64, ok bool) {
	attrs := f.Attrs()
	if len(attrs) == 0 {
		return 0, 0, false
	}
	ov, found := crossSibling(f, attrs[0])
	if !found {
		return 0, 0, false
	}

	a, aok := numericValue(f.Val())
	b, bok := numericValue(ov)
	if !aok || !bok {
		return 0, 0, false
	}
	return a, b, true
}
