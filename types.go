package validator

import (
	"context"
	"reflect"
)

// FieldError is the flat unit of a validation failure.
type FieldError struct {
	Field string
	Rule  string
	// Message is the raw template ({field}/{0}/...); root resolves it.
	Message string
	// Params are the rule arguments, resolving {0},{1},... placeholders.
	Params []string
}

// Field is a value being validated, exposing sibling fields for cross-field
// rules. A Field is only valid for the duration of the rule call that receives
// it (the engine pools and reuses instances): never retain one.
type Field interface {
	// Val is the current value; absent/nil is an invalid Value.
	Val() reflect.Value
	// Attrs are the parsed rule arguments (in:a,b,c -> [a b c]). The slice is
	// shared with the compiled expression cache: read-only, never modify it.
	Attrs() []string
	// Name is the error-reporting name; for a dive element it is the bracketed
	// key ("tags[0]") on the diagnostic path.
	Name() string
	// RootData is the whole data set, not this field's value.
	RootData() any
	Context() context.Context
	// Sibling resolves another field (dotted name) relative-first then root-anchored, never itself.
	Sibling(name string) (Field, bool)
}

// Rule is a leaf boolean rule; composition (&&, ||, !) is handled by the DSL.
// Passes must be deterministic and side-effect free: the engine may evaluate a
// rule more than once for one value (fast probe + diagnostics on dive elements,
// exhaustive Errors() collection).
type Rule interface {
	Signature() string
	Passes(f Field) bool
	Message() string
}

// ErrorRule is a Rule variant returning an error; the engine prefers PassesE over Passes.
type ErrorRule interface {
	Signature() string
	PassesE(f Field) error
	Message() string
}

// Filter transforms or sanitizes an input value (trim, lower, int, ...).
type Filter interface {
	Signature() string
	Handle(val any, args ...string) (any, error)
}

// Errors reads validation failures. Pure collection; deliberately not an error
// (use Validation.Err() for nil-on-success).
type Errors interface {
	// One returns the first message overall.
	One() string
	// OneFor returns the first message for field.
	OneFor(field string) string
	// Messages returns field's rule->message map.
	Messages(field string) map[string]string
	All() map[string]map[string]string
	Has(field string) bool
	// String aggregates all messages (fields sorted), or "" when none failed.
	String() string
}

// Validation represents a single validation run over one data set. It is not
// safe for concurrent use; configure it (AddRules/AddFilters) before Validate —
// mutators after Validate have no effect on the already-computed result.
type Validation interface {
	// AddRules ANDs rule expressions onto a field's existing expression.
	AddRules(field string, rules ...string) error
	AddFilters(field string, filters ...string) error
	RemoveRules(field string, rules ...string) error
	RemoveFilters(field string, filters ...string) error
	// ClearRules/ClearFilters drop a field's whole expression/chain.
	ClearRules(field string) error
	ClearFilters(field string) error
	Rules() map[string]string
	Filters() map[string]string
	// Bind writes the ORIGINAL data to ptr without requiring Validate.
	Bind(ptr any) error
	// SafeBind writes the FILTERED data to ptr; errors unless Validate passed.
	SafeBind(ptr any) error
	// Validate must run before accessing errors.
	Validate(ctx context.Context)
	// Errors is ALWAYS non-nil and not an error; never compare it to nil.
	Errors() Errors
	// Err is nil on success, else the failures.
	Err() error
	Fails() bool
}
