package validator

import "slices"

var (
	builtinRules   []Rule
	builtinFilters []Filter
)

// Rules returns a defensive copy of the built-in boolean rules.
func Rules() []Rule { return slices.Clone(builtinRules) }

// Filters returns the built-in value filters.
func Filters() []Filter { return slices.Clone(builtinFilters) }

func registerRules(rs ...Rule)     { builtinRules = append(builtinRules, rs...) }
func registerFilters(fs ...Filter) { builtinFilters = append(builtinFilters, fs...) }
