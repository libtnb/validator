package validator

import (
	"context"
	"net/url"
	"sync"
)

var defaultValidator = sync.OnceValue(func() *Validator { return MustNew() })

// Default returns the shared Validator used by package-level helpers.
func Default() *Validator {
	return defaultValidator()
}

// Struct prepares a struct-tag validation.
func Struct(data any) (*Validation, error) { return Default().Struct(data) }

// MustStruct is Struct with panic-on-error semantics.
func MustStruct(data any) *Validation { return Default().MustStruct(data) }

// Valid reports whether data passes its struct-tag rules.
func Valid(ctx context.Context, data any) (bool, error) { return Default().Valid(ctx, data) }

// Map prepares a Validation for any map type and explicit rules.
func Map[M ~map[K]V, K comparable, V any](data M, rules map[string]string) (*Validation, error) {
	return Default().Map(data, rules)
}

// MustMap is Map with panic-on-error semantics.
func MustMap[M ~map[K]V, K comparable, V any](data M, rules map[string]string) *Validation {
	return Default().MustMap(data, rules)
}

// JSON prepares a Validation from one JSON object and explicit rules.
func JSON[Bytes ~[]byte | ~string](data Bytes, rules map[string]string) (*Validation, error) {
	return Default().JSON(data, rules)
}

// MustJSON is JSON with panic-on-error semantics.
func MustJSON[Bytes ~[]byte | ~string](data Bytes, rules map[string]string) *Validation {
	return Default().MustJSON(data, rules)
}

// Values prepares a Validation from the first value of each form key.
func Values(data url.Values, rules map[string]string) (*Validation, error) {
	return Default().Values(data, rules)
}

// MustValues is Values with panic-on-error semantics.
func MustValues(data url.Values, rules map[string]string) *Validation {
	return Default().MustValues(data, rules)
}

// Value prepares a Validation for one value under the field name "value".
func Value(value any, rule string) (*Validation, error) { return Default().Value(value, rule) }

// MustValue is Value with panic-on-error semantics.
func MustValue(value any, rule string) *Validation { return Default().MustValue(value, rule) }

// Check reports every invalid rule tag on T; see Validator.Check.
func Check[T any]() error { return Default().Check[T]() }
