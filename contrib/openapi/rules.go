package openapi

import (
	"math"
	"strconv"

	"github.com/libtnb/validator"
)

// formats maps format-shaped rules onto OpenAPI string formats. Values outside
// the registered set (semver, ulid, ...) are legal JSON Schema annotations.
var formats = map[string]string{
	"email":    "email",
	"url":      "uri",
	"uri":      "uri",
	"uuid":     "uuid",
	"ip":       "ip",
	"ipv4":     "ipv4",
	"ipv6":     "ipv6",
	"hostname": "hostname",
	"fqdn":     "hostname",
	"date":     "date",
	"datetime": "date-time",
	"e164":     "e164",
	"semver":   "semver",
	"ulid":     "ulid",
	"jwt":      "jwt",
	"hexcolor": "hexcolor",
	"timezone": "timezone",
}

// applyRules translates flattened validate rules into schema constraints and
// reports whether the field is required. Rules with no JSON Schema counterpart
// (database rules, cross-field rules, ...) are skipped: an honest gap beats a
// wrong constraint.
func applyRules(s *Schema, rules []validator.RuleInfo) (required bool) {
	// numeric/number on a string field flips the validator's size family from
	// rune length to numeric value (numericHint). JSON Schema cannot bound the
	// numeric value of a string, so size rules are dropped and the numeric
	// shape is expressed as a pattern instead — emitting minLength for gte:18
	// would demand an 18-character string.
	numericStr := false
	if s.Type == "string" {
		for _, r := range rules {
			if r.Name == "numeric" || r.Name == "number" {
				numericStr = true
				break
			}
		}
	}
	skipSize := func() bool { return numericStr && s.Type == "string" }

	for _, rule := range rules {
		switch rule.Name {
		case "required":
			required = true
		case "filled", "notblank":
			required = true
			if s.Type == "string" && !numericStr {
				s.MinLength = uintPtr(1)
			}
		case "min", "gte":
			if !skipSize() {
				setLower(s, arg(rule, 0), false)
			}
		case "max", "lte":
			if !skipSize() {
				setUpper(s, arg(rule, 0), false)
			}
		case "gt":
			if !skipSize() {
				setLower(s, arg(rule, 0), true)
			}
		case "lt":
			if !skipSize() {
				setUpper(s, arg(rule, 0), true)
			}
		case "between":
			if !skipSize() {
				setLower(s, arg(rule, 0), false)
				setUpper(s, arg(rule, 1), false)
			}
		case "len", "size":
			if !skipSize() {
				setLower(s, arg(rule, 0), false)
				setUpper(s, arg(rule, 0), false)
			}
		case "numeric":
			if s.Type == "string" {
				s.Pattern = `^[+-]?[0-9]+(\.[0-9]+)?$`
			}
		case "number":
			if s.Type == "string" {
				s.Pattern = `^[+-]?[0-9]+$`
			}
		case "digits":
			if s.Type == "string" {
				if _, err := strconv.ParseUint(arg(rule, 0), 10, 32); err == nil {
					s.Pattern = "^[0-9]{" + arg(rule, 0) + "}$"
				}
			}
		case "in", "in_ci":
			s.Enum = enumValues(rule.Args, s.Type)
		case "regex":
			s.Pattern = arg(rule, 0)
		case "unique":
			if s.Type == "array" {
				s.UniqueItems = true
			}
		case "port":
			numericRange(s, 1, 65535)
		case "latitude":
			numericRange(s, -90, 90)
		case "longitude":
			numericRange(s, -180, 180)
		default:
			if format, ok := formats[rule.Name]; ok && s.Type == "string" {
				s.Format = format
			}
		}
	}

	return required
}

// setLower applies a lower bound: by value for numbers, by length for strings
// and arrays. JSON Schema has no exclusive length bound, so an exclusive
// length becomes the next admissible integer (len > 3 ⇒ minLength 4), and a
// fractional inclusive bound rounds toward the admissible side.
func setLower(s *Schema, raw string, exclusive bool) {
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return
	}
	switch s.Type {
	case "integer", "number":
		if exclusive {
			s.ExclusiveMinimum = &n
		} else {
			s.Minimum = &n
		}
	case "string", "array":
		if exclusive {
			n = math.Floor(n) + 1
		} else {
			n = math.Ceil(n)
		}
		if n < 0 {
			n = 0
		}
		if s.Type == "string" {
			s.MinLength = uintPtr(uint64(n))
		} else {
			s.MinItems = uintPtr(uint64(n))
		}
	}
}

func setUpper(s *Schema, raw string, exclusive bool) {
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return
	}
	switch s.Type {
	case "integer", "number":
		if exclusive {
			s.ExclusiveMaximum = &n
		} else {
			s.Maximum = &n
		}
	case "string", "array":
		if exclusive {
			n = math.Ceil(n) - 1
		} else {
			n = math.Floor(n)
		}
		if n < 0 {
			n = 0
		}
		if s.Type == "string" {
			s.MaxLength = uintPtr(uint64(n))
		} else {
			s.MaxItems = uintPtr(uint64(n))
		}
	}
}

// numericRange sets an inclusive range on numeric schemas (port, latitude, ...).
func numericRange(s *Schema, lo, hi float64) {
	if s.Type != "integer" && s.Type != "number" {
		return
	}
	s.Minimum = &lo
	s.Maximum = &hi
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

// applyElementRules applies dive-element rules to a collection schema's
// element position: items for arrays, additionalProperties for maps.
func applyElementRules(prop *Schema, element []validator.RuleInfo) {
	switch {
	case prop.Items != nil:
		applyRules(prop.Items, element)
	case prop.AdditionalProperties != nil:
		applyRules(prop.AdditionalProperties, element)
	}
}

func arg(rule validator.RuleInfo, n int) string {
	if n < len(rule.Args) {
		return rule.Args[n]
	}
	return ""
}

func uintPtr(n uint64) *uint64 { return &n }
