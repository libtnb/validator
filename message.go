package validator

import (
	"strings"
)

// AddMessages implements Validation; copies and merges the input.
func (vd *validation) AddMessages(messages map[string]string) error {
	if len(messages) == 0 {
		return nil
	}
	vd.errors.override = mergeStrMap(vd.errors.override, messages)
	return nil
}

// resolveMessage resolves fresh per read from immutable state (race-free).
func (v *Validator) resolveMessage(e FieldError, base string, override map[string]string) string {
	// suffix from explicit base, not bracket parsing: a literal name with brackets is not a dive element.
	suffix := strings.TrimPrefix(e.Field, base)
	tmpl := e.Message
	if e.Rule != "" {
		if t, ok := v.lookupTemplate(override, e.Field, base, e.Rule); ok {
			tmpl = t
		}
	}
	field := v.attrName(e.Field)
	if suffix != "" && field == e.Field {
		// no alias for the indexed key: use base alias plus suffix.
		field = v.attrName(base) + suffix
	}
	msg := substitutePlaceholders(tmpl, field, e.Params)
	if msg == "" {
		// empty template still needs an explainable failure.
		msg = substitutePlaceholders("The {field} is invalid.", field, e.Params)
	}
	return v.applyTransformFunc(msg)
}

// applyTransformFunc recovers a panicking transformFunc (else fatal panic under WithParallel).
func (v *Validator) applyTransformFunc(msg string) (out string) {
	if v.transformFunc == nil {
		return msg
	}
	defer func() {
		if recover() != nil {
			out = msg
		}
	}()
	return v.transformFunc(msg)
}

// lookupTemplate resolves a template: override > messages > translation; within
// each, "field.rule" > "base.rule" > "rule".
func (v *Validator) lookupTemplate(override map[string]string, field, base, rule string) (string, bool) {
	for _, m := range []map[string]string{override, v.messages, v.translation} {
		if m == nil {
			continue
		}
		if t, ok := m[field+"."+rule]; ok {
			return t, true
		}
		if base != field {
			if t, ok := m[base+"."+rule]; ok {
				return t, true
			}
		}
		if t, ok := m[rule]; ok {
			return t, true
		}
	}
	if v.translatorFn != nil {
		if t, ok := v.translatorFn(rule); ok {
			return t, true
		}
	}
	return "", false
}

func (v *Validator) attrName(field string) string {
	if a, ok := v.attributes[field]; ok {
		return a
	}
	return field
}

// substitutePlaceholders replaces {field}, {N}, and {N+} in one pass so a
// substituted value is never re-expanded ({N+} joins params[N:] with ", ").
// Hand-rolled: a per-message strings.NewReplacer costs ~15 allocs.
func substitutePlaceholders(tmpl, field string, params []string) string {
	if tmpl == "" || !strings.Contains(tmpl, "{") {
		return tmpl
	}
	var b strings.Builder
	b.Grow(len(tmpl) + len(field))
	for i := 0; i < len(tmpl); {
		if tmpl[i] != '{' {
			b.WriteByte(tmpl[i])
			i++
			continue
		}
		end := strings.IndexByte(tmpl[i:], '}')
		if end < 0 {
			b.WriteString(tmpl[i:])
			break
		}
		key := tmpl[i+1 : i+end]
		if key == "field" {
			b.WriteString(field)
			i += end + 1
			continue
		}
		nk, join := key, false
		if strings.HasSuffix(nk, "+") {
			nk, join = nk[:len(nk)-1], true
		}
		if n, ok := placeholderIndex(nk); ok && n < len(params) {
			if join {
				b.WriteString(strings.Join(params[n:], ", "))
			} else {
				b.WriteString(params[n])
			}
			i += end + 1
			continue
		}
		// unknown/out-of-range placeholder stays literal
		b.WriteByte('{')
		i++
	}
	return b.String()
}

// placeholderIndex parses a bare digit run ({0}, {12}); signs and other
// spellings are not placeholders.
func placeholderIndex(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
		n = n*10 + int(s[i]-'0')
		if n > 1<<20 {
			return 0, false
		}
	}
	return n, true
}
