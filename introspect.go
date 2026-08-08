package validator

import (
	"reflect"
	"strings"

	"github.com/libtnb/validator/internal/dsl"
)

// RuleInfo is one rule invocation in a field's expression, e.g. min:3.
type RuleInfo struct {
	Name string
	Args []string
}

// FieldRules lists the rules declared on one struct field, flattened from
// the top-level AND chain of its expression. Element holds the rules that
// apply to container elements (after dive). Rules under || or ! cannot be
// flattened losslessly: those subtrees are omitted and Exact reports false.
type FieldRules struct {
	// Name is the field's validation name, dotted for nested fields.
	Name string
	// Index locates the field for reflect.Type.FieldByIndex.
	Index []int
	// Rules apply to the field value itself.
	Rules []RuleInfo
	// Element rules apply to slice/array/map elements (dive).
	Element []RuleInfo
	// Exact is false when || or ! branches were omitted.
	Exact bool
}

// Describe reports the validation rules declared on T,
// for consumers that translate them into another representation — an OpenAPI
// generator mapping min:3 to minLength, for instance. Unknown rules are
// reported as-is rather than rejected; pair with Check to catch typos.
func (v *Validator) Describe[T any]() ([]FieldRules, error) {
	t, err := validationStructType[T]("Describe")
	if err != nil {
		return nil, &dsl.ParseError{Msg: err.Error()}
	}
	return v.DescribeType(t)
}

// DescribeType is the reflection-oriented form of Describe for schema
// generators that discover nested types dynamically.
func (v *Validator) DescribeType(t reflect.Type) ([]FieldRules, error) {
	if t == nil {
		return nil, &dsl.ParseError{Msg: "validator: DescribeType requires a struct type"}
	}
	t = derefType(t)
	if t.Kind() != reflect.Struct {
		return nil, &dsl.ParseError{Msg: "validator: DescribeType requires a struct type"}
	}

	sp := v.getStructPlan(t)
	out := make([]FieldRules, 0, len(sp.entries))
	for _, fp := range sp.entries {
		// non-leaf entries (struct-valued fields, embedded structs) carry rules
		// too — `Profile *Profile validate:"required"` is enforced at runtime,
		// so filtering on leaf would hide it from consumers.
		if fp.dead || strings.TrimSpace(fp.rules) == "" {
			continue
		}

		fr := FieldRules{Name: fp.name, Index: fp.index, Exact: true}

		container, element, hasDive, err := dsl.SplitDive(fp.rules, v.isRawArg)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(container) != "" {
			node, err := dsl.Parse(container, v.isRawArg)
			if err != nil {
				return nil, err
			}
			fr.Exact = flattenRules(node, &fr.Rules) && fr.Exact
		}
		if hasDive && strings.TrimSpace(element) != "" {
			node, err := dsl.Parse(element, v.isRawArg)
			if err != nil {
				return nil, err
			}
			fr.Exact = flattenRules(node, &fr.Element) && fr.Exact
		}

		out = append(out, fr)
	}

	return out, nil
}

// Describe calls Validator.Describe on the package-level default.
func Describe[T any]() ([]FieldRules, error) {
	return Default().Describe[T]()
}

// flattenRules collects the leaves of a top-level AND chain; it reports false
// when an || or ! subtree had to be skipped.
func flattenRules(n dsl.Node, rules *[]RuleInfo) bool {
	switch x := n.(type) {
	case dsl.Leaf:
		*rules = append(*rules, RuleInfo{Name: x.Name, Args: x.Args})
		return true
	case dsl.And:
		left := flattenRules(x.L, rules)
		return flattenRules(x.R, rules) && left
	default: // Or, Not: not expressible as a flat rule list
		return false
	}
}
