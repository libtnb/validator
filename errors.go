package validator

import (
	"sort"
	"strings"
)

var (
	_ Errors = (*validationErrors)(nil)
	_ error  = (*validationErrors)(nil)
)

// Messages resolve lazily and fresh on read (no shared cache): race-free.
type validationErrors struct {
	items []FieldError
	bases []string
	v     *Validator
}

func (e *validationErrors) add(fe FieldError, base string) {
	e.items = appendGrown(e.items, fe)
	e.bases = appendGrown(e.bases, base)
}

func (e *validationErrors) message(i int) string {
	if e.v == nil {
		return e.items[i].Message
	}
	return e.v.resolveMessage(e.items[i], e.bases[i])
}

func (e *validationErrors) build() map[string]map[string]string {
	m := make(map[string]map[string]string, len(e.items))
	for i := range e.items {
		inner, ok := m[e.items[i].Field]
		if !ok {
			inner = make(map[string]string)
			m[e.items[i].Field] = inner
		}
		// keep first message per (field,rule) for stability
		if _, exists := inner[e.items[i].Rule]; !exists {
			inner[e.items[i].Rule] = e.message(i)
		}
	}
	return m
}

func (e *validationErrors) One() string {
	if len(e.items) > 0 {
		return e.message(0)
	}
	return ""
}

func (e *validationErrors) OneFor(field string) string {
	for i := range e.items {
		if e.items[i].Field == field {
			return e.message(i)
		}
	}
	return ""
}

func (e *validationErrors) Messages(field string) map[string]string {
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

func (e *validationErrors) All() map[string]map[string]string { return e.build() }

func (e *validationErrors) Has(field string) bool {
	for _, it := range e.items {
		if it.Field == field {
			return true
		}
	}
	return false
}

func (e *validationErrors) String() string { return e.Error() }

func (e *validationErrors) Error() string {
	if len(e.items) == 0 {
		return ""
	}
	seen := make(map[string]bool, len(e.items))
	fields := make([]string, 0, len(e.items))
	for _, it := range e.items {
		if !seen[it.Field] {
			seen[it.Field] = true
			fields = append(fields, it.Field)
		}
	}
	sort.Strings(fields)

	var b strings.Builder
	for i, f := range fields {
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(f)
		b.WriteString(": ")
		b.WriteString(e.OneFor(f))
	}
	return b.String()
}

// appendGrown appends, using first-allocation cap 4 to skip the 1->2->4 realloc ladder.
func appendGrown[T any](dst []T, v T) []T {
	if cap(dst) == 0 {
		dst = make([]T, 0, 4)
	}
	return append(dst, v)
}
