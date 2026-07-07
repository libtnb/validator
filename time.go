package validator

import (
	"reflect"
	"strings"
	"time"

	"github.com/libtnb/validator/conv"
)

func init() {
	registerRules(
		&afterRule{},
		&afterOrEqualRule{},
		&beforeRule{},
		&beforeOrEqualRule{},
	)
}

var (
	_ Rule = (*afterRule)(nil)
	_ Rule = (*afterOrEqualRule)(nil)
	_ Rule = (*beforeRule)(nil)
	_ Rule = (*beforeOrEqualRule)(nil)
)

// afterRule: value is a date after the arg (sibling field first, else literal).
type afterRule struct{}

func (r *afterRule) Signature() string { return "after" }

func (r *afterRule) Passes(f Field) bool {
	return timeCompare(f, func(c int) bool { return c > 0 })
}

func (r *afterRule) Message() string { return "The {field} must be a date after {0}." }

// afterOrEqualRule: value is a date at or after the arg.
type afterOrEqualRule struct{}

func (r *afterOrEqualRule) Signature() string { return "after_or_equal" }

func (r *afterOrEqualRule) Passes(f Field) bool {
	return timeCompare(f, func(c int) bool { return c >= 0 })
}

func (r *afterOrEqualRule) Message() string {
	return "The {field} must be a date after or equal to {0}."
}

// beforeRule: value is a date before the arg.
type beforeRule struct{}

func (r *beforeRule) Signature() string { return "before" }

func (r *beforeRule) Passes(f Field) bool {
	return timeCompare(f, func(c int) bool { return c < 0 })
}

func (r *beforeRule) Message() string { return "The {field} must be a date before {0}." }

// beforeOrEqualRule: value is a date at or before the arg.
type beforeOrEqualRule struct{}

func (r *beforeOrEqualRule) Signature() string { return "before_or_equal" }

func (r *beforeOrEqualRule) Passes(f Field) bool {
	return timeCompare(f, func(c int) bool { return c <= 0 })
}

func (r *beforeOrEqualRule) Message() string {
	return "The {field} must be a date before or equal to {0}."
}

// timeCompare three-way compares the field's time against the arg. An arg that
// parses as a date is ALWAYS the literal bound — map/JSON keys are
// user-controlled, so a sibling lookup first would let an attacker shadow a
// hard-coded cutoff ("2026-01-01": ...) — otherwise it resolves as a sibling
// field. Empty self passes (omitempty); an unparseable side fails closed.
func timeCompare(f Field, ok func(c int) bool) bool {
	rv := f.Val()
	if isEmptyV(rv) {
		return true
	}
	attrs := f.Attrs()
	if len(attrs) == 0 {
		return false
	}
	self, valid := timeValue(rv)
	if !valid {
		return false
	}
	var other time.Time
	if lit, isLit := parseTimeLiteral(attrs[0]); isLit {
		other = lit
	} else if ov, found := crossSibling(f, attrs[0]); found {
		if other, valid = timeValue(ov); !valid {
			return false
		}
	} else {
		return false
	}
	return ok(self.Compare(other))
}

// timeValue extracts a time: time.Time as-is, strings/[]byte/integers via
// conv.ToTime. Empty values (blank string, zero time.Time) fail: an empty
// reference cannot anchor a chronological assertion — conv.ToTime("") returns
// the zero time with no error, which would otherwise compare against year 1.
func timeValue(rv reflect.Value) (time.Time, bool) {
	if isEmptyV(rv) {
		return time.Time{}, false
	}
	if rv.Type() == timeType {
		return rv.Interface().(time.Time), true
	}
	if !rv.CanInterface() {
		return time.Time{}, false
	}
	t, err := conv.ToTime(rv.Interface())
	if err != nil || t.IsZero() {
		return time.Time{}, false
	}
	return t, true
}

// parseTimeLiteral rejects blank input for the same reason as timeValue.
func parseTimeLiteral(s string) (time.Time, bool) {
	if strings.TrimSpace(s) == "" {
		return time.Time{}, false
	}
	t, err := conv.ToTime(s)
	if err != nil || t.IsZero() {
		return time.Time{}, false
	}
	return t, true
}
