package validator

import (
	"maps"
	"reflect"
)

// TagNameFunc derives a field's validation name from its StructField; an empty return keeps the Go field name.
type TagNameFunc func(field reflect.StructField) string

// TransformFunc is a final transform applied to each resolved error message.
type TransformFunc func(message string) string

// TranslatorFunc returns a rule's template, or false to fall through.
type TranslatorFunc func(rule string) (string, bool)

// Option configures a Validator during initialization.
type Option func(*Validator)

// WithTagName sets the struct tag used for validation rules (default "validate").
func WithTagName(name string) Option {
	return func(v *Validator) {
		v.tagName = name
	}
}

// WithTagNameFunc derives the tag name per field.
func WithTagNameFunc(fn TagNameFunc) Option {
	return func(v *Validator) {
		v.tagNameFunc = fn
	}
}

// WithTransformFunc sets a final transform over each resolved message.
func WithTransformFunc(fn TransformFunc) Option {
	return func(v *Validator) {
		v.transformFunc = fn
	}
}

// WithStrictRequired makes `required` reject zero values, not just absent/nil.
func WithStrictRequired() Option {
	return func(v *Validator) {
		v.strictRequired = true
	}
}

// WithPrivateFieldValidation enables validation of unexported fields (via unsafe).
func WithPrivateFieldValidation() Option {
	return func(v *Validator) {
		v.privateFieldValidation = true
	}
}

// WithoutBuiltinRules disables registration of the built-in rules and filters.
func WithoutBuiltinRules() Option {
	return func(v *Validator) {
		v.builtin = false
	}
}

// WithAttributes maps field names to display names for {field}.
func WithAttributes(attrs map[string]string) Option {
	return func(v *Validator) {
		v.attributes = mergeStrMap(v.attributes, attrs)
	}
}

// WithParallel validates fields concurrently once a validation has at least minFields; values < 1 disable parallelism.
func WithParallel(minFields int) Option {
	return func(v *Validator) {
		v.parallel = minFields
	}
}

// WithMessages sets message templates keyed by "field.rule" (highest priority) or "rule".
func WithMessages(messages map[string]string) Option {
	return func(v *Validator) {
		v.messages = mergeStrMap(v.messages, messages)
	}
}

// WithTranslation sets localized message templates keyed by "field.rule" or "rule".
func WithTranslation(messages map[string]string) Option {
	return func(v *Validator) {
		v.translation = mergeStrMap(v.translation, messages)
	}
}

// WithTranslator sets a function consulted after WithTranslation's map.
func WithTranslator(fn TranslatorFunc) Option {
	return func(v *Validator) {
		v.translatorFn = fn
	}
}

func mergeStrMap(dst, src map[string]string) map[string]string {
	out := make(map[string]string, len(dst)+len(src))
	maps.Copy(out, dst)
	maps.Copy(out, src)
	return out
}
