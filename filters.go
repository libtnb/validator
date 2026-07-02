package validator

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/libtnb/validator/conv"
)

func init() {
	registerFilters(
		&trimFilter{},
		&ltrimFilter{},
		&rtrimFilter{},
		&lowerFilter{},
		&upperFilter{},
		&titleFilter{},
		&intFilter{},
		&floatFilter{},
		&boolFilter{},
		&stringFilter{},
	)
}

var (
	_ Filter = (*trimFilter)(nil)
	_ Filter = (*ltrimFilter)(nil)
	_ Filter = (*rtrimFilter)(nil)
	_ Filter = (*lowerFilter)(nil)
	_ Filter = (*upperFilter)(nil)
	_ Filter = (*titleFilter)(nil)
	_ Filter = (*intFilter)(nil)
	_ Filter = (*floatFilter)(nil)
	_ Filter = (*boolFilter)(nil)
	_ Filter = (*stringFilter)(nil)
)

// trimFilter trims surrounding whitespace (or the arg cutset).
type trimFilter struct{}

func (f *trimFilter) Signature() string { return "trim" }

func (f *trimFilter) Handle(val any, args ...string) (any, error) {
	s := conv.ToString(val)
	if len(args) > 0 {
		return strings.Trim(s, args[0]), nil
	}
	return strings.TrimSpace(s), nil
}

// ltrimFilter trims leading whitespace (or the arg cutset).
type ltrimFilter struct{}

func (f *ltrimFilter) Signature() string { return "ltrim" }

func (f *ltrimFilter) Handle(val any, args ...string) (any, error) {
	s := conv.ToString(val)
	if len(args) > 0 {
		return strings.TrimLeft(s, args[0]), nil
	}
	return strings.TrimLeftFunc(s, unicode.IsSpace), nil
}

// rtrimFilter trims trailing whitespace (or the arg cutset).
type rtrimFilter struct{}

func (f *rtrimFilter) Signature() string { return "rtrim" }

func (f *rtrimFilter) Handle(val any, args ...string) (any, error) {
	s := conv.ToString(val)
	if len(args) > 0 {
		return strings.TrimRight(s, args[0]), nil
	}
	return strings.TrimRightFunc(s, unicode.IsSpace), nil
}

// lowerFilter lower-cases the value.
type lowerFilter struct{}

func (f *lowerFilter) Signature() string { return "lower" }

func (f *lowerFilter) Handle(val any, args ...string) (any, error) {
	return strings.ToLower(conv.ToString(val)), nil
}

// upperFilter upper-cases the string form.
type upperFilter struct{}

func (f *upperFilter) Signature() string { return "upper" }

func (f *upperFilter) Handle(val any, args ...string) (any, error) {
	return strings.ToUpper(conv.ToString(val)), nil
}

// titleFilter title-cases each word, normalizing whitespace runs to single spaces.
type titleFilter struct{}

func (f *titleFilter) Signature() string { return "title" }

func (f *titleFilter) Handle(val any, args ...string) (any, error) {
	return titleCase(conv.ToString(val)), nil
}

// intFilter converts to int64.
type intFilter struct{}

func (f *intFilter) Signature() string { return "int" }

func (f *intFilter) Handle(val any, args ...string) (any, error) {
	return conv.ToInt(val)
}

// floatFilter converts to float64.
type floatFilter struct{}

func (f *floatFilter) Signature() string { return "float" }

func (f *floatFilter) Handle(val any, args ...string) (any, error) {
	return conv.ToFloat(val)
}

// boolFilter converts to bool.
type boolFilter struct{}

func (f *boolFilter) Signature() string { return "bool" }

func (f *boolFilter) Handle(val any, args ...string) (any, error) {
	return conv.ToBool(val)
}

// stringFilter converts to string.
type stringFilter struct{}

func (f *stringFilter) Signature() string { return "string" }

func (f *stringFilter) Handle(val any, args ...string) (any, error) {
	return conv.ToString(val), nil
}

func titleCase(s string) string {
	fields := strings.Fields(s)
	for i, w := range fields {
		r, size := utf8.DecodeRuneInString(w)
		if r == utf8.RuneError {
			continue
		}
		fields[i] = string(unicode.ToUpper(r)) + strings.ToLower(w[size:])
	}
	return strings.Join(fields, " ")
}
