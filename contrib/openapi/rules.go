package openapi

import (
	"reflect"
	"strconv"

	"github.com/libtnb/validator"
)

// formats maps format-shaped rules onto OpenAPI string formats.
var formats = map[string]string{
	"email":    "email",
	"url":      "uri",
	"uri":      "uri",
	"uuid":     "uuid",
	"ip":       "ip",
	"ipv4":     "ipv4",
	"ipv6":     "ipv6",
	"hostname": "hostname",
	"date":     "date",
	"datetime": "date-time",
}

// applyRules translates flattened validate rules into schema constraints and
// reports whether the field is required. Rules with no JSON Schema
// counterpart (database rules, cross-field rules, ...) are skipped.
func applyRules(s *Schema, rules []validator.RuleInfo, t reflect.Type) (required bool) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	for _, rule := range rules {
		switch rule.Name {
		case "required":
			required = true
		case "filled", "notblank":
			required = true
			if s.Type == "string" {
				s.MinLength = uintPtr(1)
			}
		case "min", "gte":
			setLower(s, t, arg(rule, 0), false)
		case "max", "lte":
			setUpper(s, t, arg(rule, 0), false)
		case "gt":
			setLower(s, t, arg(rule, 0), true)
		case "lt":
			setUpper(s, t, arg(rule, 0), true)
		case "between":
			setLower(s, t, arg(rule, 0), false)
			setUpper(s, t, arg(rule, 1), false)
		case "len", "size":
			setLower(s, t, arg(rule, 0), false)
			setUpper(s, t, arg(rule, 0), false)
		case "in", "in_ci":
			s.Enum = enumValues(rule.Args, s.Type)
		case "regex":
			s.Pattern = arg(rule, 0)
		default:
			if format, ok := formats[rule.Name]; ok && s.Type == "string" {
				s.Format = format
			}
		}
	}

	return required
}

// setLower applies a numeric lower bound by value for numbers, by length for
// strings, arrays and maps — mirroring the validator's size semantics.
func setLower(s *Schema, t reflect.Type, raw string, exclusive bool) {
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return
	}
	switch {
	case s.Type == "integer" || s.Type == "number":
		if exclusive {
			s.ExclusiveMinimum = &n
		} else {
			s.Minimum = &n
		}
	case s.Type == "string":
		s.MinLength = uintPtr(uint64(n))
	case s.Type == "array":
		s.MinItems = uintPtr(uint64(n))
	}
	_ = t
}

func setUpper(s *Schema, t reflect.Type, raw string, exclusive bool) {
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return
	}
	switch {
	case s.Type == "integer" || s.Type == "number":
		if exclusive {
			s.ExclusiveMaximum = &n
		} else {
			s.Maximum = &n
		}
	case s.Type == "string":
		s.MaxLength = uintPtr(uint64(n))
	case s.Type == "array":
		s.MaxItems = uintPtr(uint64(n))
	}
	_ = t
}

// enumValues keeps enum members typed: numeric schemas get numbers.
func enumValues(args []string, schemaType string) []any {
	out := make([]any, 0, len(args))
	for _, a := range args {
		if schemaType == "integer" || schemaType == "number" {
			if n, err := strconv.ParseFloat(a, 64); err == nil {
				out = append(out, n)
				continue
			}
		}
		out = append(out, a)
	}
	return out
}

func arg(rule validator.RuleInfo, n int) string {
	if n < len(rule.Args) {
		return rule.Args[n]
	}
	return ""
}

func uintPtr(n uint64) *uint64 { return &n }
