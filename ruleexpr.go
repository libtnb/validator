package validator

import (
	"errors"
	"fmt"
	"maps"
	"strings"

	"github.com/libtnb/validator/internal/dsl"
)

// AddRules joins expressions with AND. Invalid expressions leave rules unchanged.
func (vd *Validation) AddRules(field string, rules ...string) error {
	if err := vd.ensureMutable(); err != nil {
		return err
	}
	candidate := vd.rules[field]
	for _, r := range rules {
		if strings.TrimSpace(r) == "" {
			continue
		}
		candidate = vd.mergeRule(candidate, r)
	}
	if candidate == vd.rules[field] {
		return nil
	}
	if err := vd.checkExpr(candidate); err != nil {
		return err
	}
	vd.ensureOwnedRules()
	vd.rules[field] = candidate
	return nil
}

// RemoveRules removes named top-level rules. No names is a no-op.
func (vd *Validation) RemoveRules(field string, rules ...string) error {
	if err := vd.ensureMutable(); err != nil {
		return err
	}
	if len(rules) == 0 {
		return nil
	}
	cur, ok := vd.rules[field]
	if !ok {
		return nil
	}
	names := make(map[string]bool, len(rules))
	for _, r := range rules {
		names[strings.TrimSpace(r)] = true
	}
	vd.ensureOwnedRules()
	if nw := dsl.RemoveTopLevelLeaves(cur, names, vd.validator.isRawArg); strings.TrimSpace(nw) == "" {
		delete(vd.rules, field)
	} else {
		vd.rules[field] = nw
	}
	return nil
}

// ClearRules drops a field's entire rule expression.
func (vd *Validation) ClearRules(field string) error {
	if err := vd.ensureMutable(); err != nil {
		return err
	}
	if _, ok := vd.rules[field]; ok {
		vd.ensureOwnedRules()
		delete(vd.rules, field)
	}
	return nil
}

// Rules returns a copy of the field->expression map.
func (vd *Validation) Rules() map[string]string { return copyStrMap(vd.rules) }

// ensureOwnedRules copy-on-writes vd.rules before mutation: never corrupt the shared cached map.
func (vd *Validation) ensureOwnedRules() {
	if vd.rulesShared || vd.rules == nil {
		vd.rules = copyStrMap(vd.rules)
		vd.rulesShared = false
	}
}

// mergeRule adds r to the container side of a dive expression.
func (vd *Validation) mergeRule(existing, r string) string {
	if strings.TrimSpace(existing) == "" {
		return r
	}
	if ds := vd.validator.splitDive(existing); ds.err == nil && ds.hasDive {
		// element is never empty here: SplitDive rejects a trailing dive.
		container := ds.container
		if container == "" {
			container = r
		} else {
			container = vd.joinAnd(container, r)
		}
		return container + " && dive && " + ds.element
	}
	return vd.joinAnd(existing, r)
}

func (vd *Validation) checkExpr(expr string) error {
	ds := vd.validator.splitDive(expr)
	if ds.err != nil {
		return ds.err
	}
	if ds.nested {
		return fmt.Errorf("validator: multiple top-level 'dive' is not supported")
	}
	if strings.TrimSpace(ds.container) != "" {
		if _, err := vd.validator.compile(ds.container); err != nil {
			return err
		}
	}
	if ds.hasDive && strings.TrimSpace(ds.element) != "" {
		e, err := vd.validator.compile(ds.element)
		if err != nil {
			return err
		}
		if e.sometimes {
			return errors.New(errSometimesInDive)
		}
	}
	return nil
}

// joinAnd ANDs existing and r, parenthesizing a top-level || side so && (tighter) doesn't capture it.
func (vd *Validation) joinAnd(existing, r string) string {
	if dsl.HasTopLevelOr(existing, vd.validator.isRawArg) {
		existing = "(" + escapeTrailingBackslash(existing) + ")"
	}
	if dsl.HasTopLevelOr(r, vd.validator.isRawArg) {
		r = "(" + escapeTrailingBackslash(r) + ")"
	}
	return existing + " && " + r
}

// escapeTrailingBackslash doubles a trailing odd backslash run so a following
// structural ")" can't be swallowed as an escape.
func escapeTrailingBackslash(s string) string {
	n := 0
	for i := len(s) - 1; i >= 0 && s[i] == '\\'; i-- {
		n++
	}
	if n%2 == 1 {
		return s + "\\"
	}
	return s
}

func copyStrMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	maps.Copy(out, m)
	return out
}
