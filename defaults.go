package validator

import (
	"net/url"
	"sync"
)

// defaultValidator is lazy: eager init would run before rule init() fills the catalog.
var defaultValidator = sync.OnceValue(func() *Validator { return NewValidator() })

// Default returns the shared Validator behind the package funcs; mutating it is process-global.
func Default() *Validator { return defaultValidator() }

func Struct(data any) Validation { return defaultValidator().Struct(data) }

// Valid reports whether data passes its struct-tag rules on the allocation-free
// fast path; data carrying no rules (a plain map or scalar) is vacuously valid.
// Use Struct when you need error details, Bind, or to add rules.
func Valid(data any) bool { return defaultValidator().Valid(data) }

func Map(data map[string]any, rules map[string]string) Validation {
	return defaultValidator().Map(data, rules)
}

func JSON(data string, rules map[string]string) Validation {
	return defaultValidator().JSON(data, rules)
}

func URLValues(data url.Values, rules map[string]string) Validation {
	return defaultValidator().URLValues(data, rules)
}

func Any(data any) Validation { return defaultValidator().Any(data) }

func Var(value any, rule string) Validation { return defaultValidator().Var(value, rule) }
