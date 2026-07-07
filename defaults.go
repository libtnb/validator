package validator

import (
	"net/url"
	"sync/atomic"
)

// defaultValidator is lazy: eager init would run before rule init() fills the
// catalog. It holds nil until the first Default() call or until SetDefault
// installs a configured instance.
var defaultValidator atomic.Pointer[Validator]

// Default returns the shared Validator behind the package funcs; mutating it
// is process-global. It is created on first use unless SetDefault installed
// one earlier.
func Default() *Validator {
	if v := defaultValidator.Load(); v != nil {
		return v
	}
	// racing first callers may each build one; exactly one wins the publish
	v := NewValidator()
	if defaultValidator.CompareAndSwap(nil, v) {
		return v
	}
	return defaultValidator.Load()
}

// SetDefault makes v the Validator returned by Default and used by the
// package-level helpers (Struct, Map, JSON, ...). Like slog.SetDefault it is
// meant to be called once during startup, before concurrent use — typically
// with an instance carrying custom rules, messages and translations, so the
// rest of the program can validate through the package funcs without passing
// a *Validator around.
func SetDefault(v *Validator) {
	if v == nil {
		panic("validator: SetDefault(nil)")
	}
	defaultValidator.Store(v)
}

func Struct(data any) Validation { return Default().Struct(data) }

// Valid reports whether data passes its struct-tag rules on the allocation-free
// fast path; data carrying no rules (a plain map or scalar) is vacuously valid.
// Use Struct when you need error details, Bind, or to add rules.
func Valid(data any) bool { return Default().Valid(data) }

func Map(data map[string]any, rules map[string]string) Validation {
	return Default().Map(data, rules)
}

func JSON(data string, rules map[string]string) Validation {
	return Default().JSON(data, rules)
}

func URLValues(data url.Values, rules map[string]string) Validation {
	return Default().URLValues(data, rules)
}

func Any(data any) Validation { return Default().Any(data) }

func Var(value any, rule string) Validation { return Default().Var(value, rule) }

// CheckRules reports every bad rule tag on data's struct type; see Validator.CheckRules.
func CheckRules(data any) error { return Default().CheckRules(data) }
