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

type alphaRule struct{}

func (r *alphaRule) Signature() string { return "alpha" }

func (r *alphaRule) Passes(f *Field) bool {
	return runesAllPass(f, unicode.IsLetter)
}

func (r *alphaRule) Message() string { return "The {field} field must only contain letters." }

type alphanumRule struct{}

func (r *alphanumRule) Signature() string { return "alphanum" }

func (r *alphanumRule) Passes(f *Field) bool {
	return runesAllPass(f, func(c rune) bool { return unicode.IsLetter(c) || unicode.IsDigit(c) })
}

func (r *alphanumRule) Message() string {
	return "The {field} field must only contain letters and numbers."
}

type asciiRule struct{}

func (r *asciiRule) Signature() string { return "ascii" }

func (r *asciiRule) Passes(f *Field) bool {
	return runesAllPass(f, func(c rune) bool { return c <= unicode.MaxASCII })
}

func (r *asciiRule) Message() string {
	return "The {field} field must only contain single-byte alphanumeric characters and symbols."
}

type lowercaseRule struct{}

func (r *lowercaseRule) Signature() string { return "lowercase" }

func (r *lowercaseRule) Passes(f *Field) bool {
	return caseMatch(f, strings.ToLower)
}

func (r *lowercaseRule) Message() string { return "The {field} field must be lowercase." }

type uppercaseRule struct{}

func (r *uppercaseRule) Signature() string { return "uppercase" }

func (r *uppercaseRule) Passes(f *Field) bool {
	return caseMatch(f, strings.ToUpper)
}

func (r *uppercaseRule) Message() string { return "The {field} field must be uppercase." }

type containsRule struct{}

func (r *containsRule) Signature() string { return "contains" }

func (r *containsRule) Passes(f *Field) bool {
	return substringMatch(f, strings.Contains)
}

func (r *containsRule) Message() string { return "The {field} field must contain {0}." }

type excludesRule struct{}

func (r *excludesRule) Signature() string { return "excludes" }

func (r *excludesRule) Passes(f *Field) bool {
	return substringMatch(f, func(s, arg string) bool { return !strings.Contains(s, arg) })
}

func (r *excludesRule) Message() string { return "The {field} field must not contain {0}." }

type startsWithRule struct{}

func (r *startsWithRule) Signature() string { return "startswith" }

func (r *startsWithRule) Passes(f *Field) bool {
	return substringMatch(f, strings.HasPrefix)
}

func (r *startsWithRule) Message() string { return "The {field} field must start with {0}." }

type endsWithRule struct{}

func (r *endsWithRule) Signature() string { return "endswith" }

func (r *endsWithRule) Passes(f *Field) bool {
	return substringMatch(f, strings.HasSuffix)
}

func (r *endsWithRule) Message() string { return "The {field} field must end with {0}." }

var (
	_ leafCompiler = (*alphaRule)(nil)
	_ leafCompiler = (*alphanumRule)(nil)
	_ leafCompiler = (*asciiRule)(nil)
	_ leafCompiler = (*lowercaseRule)(nil)
	_ leafCompiler = (*uppercaseRule)(nil)
)

func (r *alphaRule) compilePasses([]string) func(*Field) bool     { return r.Passes }
func (r *alphanumRule) compilePasses([]string) func(*Field) bool  { return r.Passes }
func (r *asciiRule) compilePasses([]string) func(*Field) bool     { return r.Passes }
func (r *lowercaseRule) compilePasses([]string) func(*Field) bool { return r.Passes }
func (r *uppercaseRule) compilePasses([]string) func(*Field) bool { return r.Passes }

func runesAllPass(f *Field, ok func(rune) bool) bool {
	if isEmptyV(f.Reflect()) {
		return true
	}
	for _, c := range valString(f.Reflect()) {
		if !ok(c) {
			return false
		}
	}
	return true
}

func substringMatch(f *Field, match func(s, arg string) bool) bool {
	if isEmptyV(f.Reflect()) {
		return true
	}
	attrs := f.Attrs()
	if len(attrs) == 0 {
		return true
	}
	return match(valString(f.Reflect()), attrs[0])
}

func caseMatch(f *Field, transform func(string) string) bool {
	if isEmptyV(f.Reflect()) {
		return true
	}
	s := valString(f.Reflect())
	return s == transform(s)
}
