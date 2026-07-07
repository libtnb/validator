package validator

import (
	"reflect"
	"strings"
)

func init() { registerRules(&requiredRule{}, &filledRule{}, &notBlankRule{}, &sometimesRule{}) }

var (
	_ Rule = (*requiredRule)(nil)
	_ Rule = (*filledRule)(nil)
	_ Rule = (*notBlankRule)(nil)
	_ Rule = (*sometimesRule)(nil)

	_ leafCompiler = (*requiredRule)(nil)
	_ leafCompiler = (*filledRule)(nil)
	_ leafCompiler = (*notBlankRule)(nil)
	_ leafCompiler = (*sometimesRule)(nil)
)

// strictForm yields a strict variant, swapped in under WithStrictRequired.
type strictForm interface{ withStrict() Rule }

// requiredRule: present, non-zero under strict. Validates empties, never skips them.
type requiredRule struct{ strict bool }

func (r *requiredRule) Signature() string   { return "required" }
func (r *requiredRule) Passes(f Field) bool { return present(f, r.strict) }
func (r *requiredRule) withStrict() Rule    { return &requiredRule{strict: true} }
func (r *requiredRule) Message() string     { return "The {field} field is required." }

// compilePasses (leafCompiler fast path): hand the compiler Passes directly, skipping per-eval arg binding and dispatch.
func (r *requiredRule) compilePasses([]string) func(Field) bool { return r.Passes }

type filledRule struct{}

func (r *filledRule) Signature() string { return "filled" }

func (r *filledRule) Passes(f Field) bool { return !isEmptyV(f.Val()) }

func (r *filledRule) Message() string { return "The {field} field must not be empty." }

func (r *filledRule) compilePasses([]string) func(Field) bool { return r.Passes }

type notBlankRule struct{}

func (r *notBlankRule) Signature() string { return "notblank" }

func (r *notBlankRule) Passes(f Field) bool {
	return strings.TrimSpace(valString(f.Val())) != ""
}

func (r *notBlankRule) Message() string { return "The {field} field must not be blank." }

func (r *notBlankRule) compilePasses([]string) func(Field) bool { return r.Passes }

// sometimesRule is a marker: on the AND spine it makes the engine skip the whole
// field when the value is absent (missing key, nil pointer). PATCH semantics:
// "sometimes && required && email" validates only when the key is provided.
// As a leaf it always passes; the skip happens in the engine (see compiled.sometimes).
type sometimesRule struct{}

func (r *sometimesRule) Signature() string { return "sometimes" }

func (r *sometimesRule) Passes(Field) bool { return true }

func (r *sometimesRule) Message() string { return "The {field} is invalid." }

func (r *sometimesRule) compilePasses([]string) func(Field) bool { return r.Passes }

// present is the shared required-family check: valid and non-nil (typed-nil counts as
// absent), strict additionally requiring non-zero.
func present(f Field, strict bool) bool {
	rv := f.Val()
	if !rv.IsValid() {
		return false
	}
	if !strict {
		return true
	}
	if isEmptyV(rv) {
		return false
	}
	if k := rv.Kind(); k == reflect.Struct || k == reflect.Array {
		return !rv.IsZero()
	}
	return true
}
