package validator

// FieldError is the flat unit of a validation failure.
type FieldError struct {
	Field string
	Rule  string
	// Message is the raw template ({field}/{0}/...); root resolves it.
	Message string
	// Params are the rule arguments, resolving {0},{1},... placeholders.
	Params []string
}

// Rule is a leaf boolean rule; composition (&&, ||, !) is handled by the DSL.
// Passes must be deterministic and side-effect free: the engine may evaluate a
// rule more than once for one value (fast probe + diagnostics on dive elements,
// exhaustive Errors() collection).
type Rule interface {
	// Signature returns the name used in rule expressions.
	Signature() string
	// Passes evaluates the current field.
	Passes(f *Field) bool
	// Message returns the failure template.
	Message() string
}

// FallibleRule is a rule backed by an operation that can fail. Validate returns
// false, nil for an invalid value; a non-nil error means validation could not
// be completed and is never converted into a field failure.
type FallibleRule interface {
	// Signature returns the name used in rule expressions.
	Signature() string
	// Validate evaluates the current field.
	Validate(f *Field) (bool, error)
	// Message returns the invalid-value template.
	Message() string
}

// Filter transforms or sanitizes an input value (trim, lower, int, ...).
type Filter interface {
	// Signature returns the name used in filter chains.
	Signature() string
	// Handle transforms val with the parsed arguments.
	Handle(val any, args ...string) (any, error)
}
