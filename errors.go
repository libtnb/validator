package validator

import (
	"errors"
	"slices"
	"strings"
)

var _ error = (*Errors)(nil)

// Errors is the read-only collection produced by a Validation.
type Errors struct {
	items    []FieldError
	bases    []string
	override map[string]string
	v        *Validator
}

// One returns the first resolved message, or an empty string.
func (e *Errors) One() string {
	if len(e.items) > 0 {
		return e.message(0)
	}
	return ""
}

// OneFor returns the first resolved message for field.
func (e *Errors) OneFor(field string) string {
	for i := range e.items {
		if e.items[i].Field == field {
			return e.message(i)
		}
	}
	return ""
}

// Messages returns rule-to-message values for field.
func (e *Errors) Messages(field string) map[string]string {
	inner := make(map[string]string)
	for i := range e.items {
		if e.items[i].Field != field {
			continue
		}
		if _, exists := inner[e.items[i].Rule]; !exists {
			inner[e.items[i].Rule] = e.message(i)
		}
	}
	return inner
}

// All returns a defensive field-to-rule-to-message map.
func (e *Errors) All() map[string]map[string]string { return e.build() }

// Items returns resolved FieldError copies.
func (e *Errors) Items() []FieldError {
	out := make([]FieldError, len(e.items))
	for i := range e.items {
		it := e.items[i]
		it.Message = e.message(i)
		it.Params = slices.Clone(it.Params)
		out[i] = it
	}
	return out
}

// Has reports whether field failed validation.
func (e *Errors) Has(field string) bool {
	for _, it := range e.items {
		if it.Field == field {
			return true
		}
	}
	return false
}

// String returns the same stable summary as Error.
func (e *Errors) String() string { return e.Error() }

// Error summarizes the first message for each failed field.
func (e *Errors) Error() string {
	if len(e.items) == 0 {
		return ""
	}
	first := make(map[string]string, len(e.items))
	fields := make([]string, 0, len(e.items))
	for i := range e.items {
		f := e.items[i].Field
		if _, ok := first[f]; !ok {
			first[f] = e.message(i)
			fields = append(fields, f)
		}
	}
	slices.Sort(fields)

	var b strings.Builder
	for i, f := range fields {
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(f)
		b.WriteString(": ")
		b.WriteString(first[f])
	}
	return b.String()
}

// AsErrors finds an Errors collection through wrapped errors.
func AsErrors(err error) (*Errors, bool) {
	return errors.AsType[*Errors](err)
}

func (e *Errors) add(fe FieldError, base string) {
	e.items = appendGrown(e.items, fe)
	e.bases = appendGrown(e.bases, base)
}

func (e *Errors) message(i int) string {
	if e.v == nil {
		return e.items[i].Message
	}
	return e.v.resolveMessage(e.items[i], e.bases[i], e.override)
}

func (e *Errors) build() map[string]map[string]string {
	m := make(map[string]map[string]string, len(e.items))
	for i := range e.items {
		inner, ok := m[e.items[i].Field]
		if !ok {
			inner = make(map[string]string)
			m[e.items[i].Field] = inner
		}
		// Keep the first message per field and rule for stable output.
		if _, exists := inner[e.items[i].Rule]; !exists {
			inner[e.items[i].Rule] = e.message(i)
		}
	}
	return m
}
func appendGrown[T any](dst []T, v T) []T {
	if cap(dst) == 0 {
		dst = make([]T, 0, 4)
	}
	return append(dst, v)
}
