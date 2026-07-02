package validator

import (
	"strings"
	"unicode"
)

func init() {
	registerRules(
		&alphaRule{},
		&alphanumRule{},
		&asciiRule{},
		&lowercaseRule{},
		&uppercaseRule{},
		&containsRule{},
		&excludesRule{},
		&startsWithRule{},
		&endsWithRule{},
	)
}

var (
	_ Rule = (*alphaRule)(nil)
	_ Rule = (*alphanumRule)(nil)
	_ Rule = (*asciiRule)(nil)
	_ Rule = (*lowercaseRule)(nil)
	_ Rule = (*uppercaseRule)(nil)
	_ Rule = (*containsRule)(nil)
	_ Rule = (*excludesRule)(nil)
	_ Rule = (*startsWithRule)(nil)
	_ Rule = (*endsWithRule)(nil)
)

// alphaRule: Unicode letters only.
type alphaRule struct{}

func (r *alphaRule) Signature() string { return "alpha" }

func (r *alphaRule) Passes(f Field) bool {
	return runesAllPass(f, unicode.IsLetter)
}

func (r *alphaRule) Message() string { return "The {field} field must only contain letters." }

// alphanumRule: letters and digits only.
type alphanumRule struct{}

func (r *alphanumRule) Signature() string { return "alphanum" }

func (r *alphanumRule) Passes(f Field) bool {
	return runesAllPass(f, func(c rune) bool { return unicode.IsLetter(c) || unicode.IsDigit(c) })
}

func (r *alphanumRule) Message() string {
	return "The {field} field must only contain letters and numbers."
}

// asciiRule: 7-bit ASCII only.
type asciiRule struct{}

func (r *asciiRule) Signature() string { return "ascii" }

func (r *asciiRule) Passes(f Field) bool {
	return runesAllPass(f, func(c rune) bool { return c <= unicode.MaxASCII })
}

func (r *asciiRule) Message() string {
	return "The {field} field must only contain single-byte alphanumeric characters and symbols."
}

// lowercaseRule: value equals its lowercased form.
type lowercaseRule struct{}

func (r *lowercaseRule) Signature() string { return "lowercase" }

func (r *lowercaseRule) Passes(f Field) bool {
	return caseMatch(f, strings.ToLower)
}

func (r *lowercaseRule) Message() string { return "The {field} field must be lowercase." }

// uppercaseRule: value equals its uppercased form.
type uppercaseRule struct{}

func (r *uppercaseRule) Signature() string { return "uppercase" }

func (r *uppercaseRule) Passes(f Field) bool {
	return caseMatch(f, strings.ToUpper)
}

func (r *uppercaseRule) Message() string { return "The {field} field must be uppercase." }

// containsRule: value contains the substring arg (contains:foo).
type containsRule struct{}

func (r *containsRule) Signature() string { return "contains" }

func (r *containsRule) Passes(f Field) bool {
	return substringMatch(f, strings.Contains)
}

func (r *containsRule) Message() string { return "The {field} field must contain {0}." }

// excludesRule: value omits the substring arg (excludes:foo).
type excludesRule struct{}

func (r *excludesRule) Signature() string { return "excludes" }

func (r *excludesRule) Passes(f Field) bool {
	return substringMatch(f, func(s, arg string) bool { return !strings.Contains(s, arg) })
}

func (r *excludesRule) Message() string { return "The {field} field must not contain {0}." }

// startsWithRule: value begins with the prefix arg (startswith:foo).
type startsWithRule struct{}

func (r *startsWithRule) Signature() string { return "startswith" }

func (r *startsWithRule) Passes(f Field) bool {
	return substringMatch(f, strings.HasPrefix)
}

func (r *startsWithRule) Message() string { return "The {field} field must start with {0}." }

// endsWithRule: value ends with the suffix arg (endswith:foo).
type endsWithRule struct{}

func (r *endsWithRule) Signature() string { return "endswith" }

func (r *endsWithRule) Passes(f Field) bool {
	return substringMatch(f, strings.HasSuffix)
}

func (r *endsWithRule) Message() string { return "The {field} field must end with {0}." }

// compilePasses (leafCompiler): arg-less rules hand the compiler their Passes
// method directly, skipping per-eval arg binding and interface dispatch.
var (
	_ leafCompiler = (*alphaRule)(nil)
	_ leafCompiler = (*alphanumRule)(nil)
	_ leafCompiler = (*asciiRule)(nil)
	_ leafCompiler = (*lowercaseRule)(nil)
	_ leafCompiler = (*uppercaseRule)(nil)
)

func (r *alphaRule) compilePasses([]string) func(Field) bool     { return r.Passes }
func (r *alphanumRule) compilePasses([]string) func(Field) bool  { return r.Passes }
func (r *asciiRule) compilePasses([]string) func(Field) bool     { return r.Passes }
func (r *lowercaseRule) compilePasses([]string) func(Field) bool { return r.Passes }
func (r *uppercaseRule) compilePasses([]string) func(Field) bool { return r.Passes }

func runesAllPass(f Field, ok func(rune) bool) bool {
	if isEmptyV(f.Val()) {
		return true
	}
	for _, c := range valString(f.Val()) {
		if !ok(c) {
			return false
		}
	}
	return true
}

func substringMatch(f Field, match func(s, arg string) bool) bool {
	if isEmptyV(f.Val()) {
		return true
	}
	attrs := f.Attrs()
	if len(attrs) == 0 {
		return true
	}
	return match(valString(f.Val()), attrs[0])
}

func caseMatch(f Field, transform func(string) string) bool {
	if isEmptyV(f.Val()) {
		return true
	}
	s := valString(f.Val())
	return s == transform(s)
}
