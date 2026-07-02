package validator

var (
	builtinRules      []Rule
	builtinErrorRules []ErrorRule
	builtinFilters    []Filter
)

// Rules returns a copy of the catalog; the copy guards the shared backing slice.
func Rules() []Rule           { return append([]Rule(nil), builtinRules...) }
func ErrorRules() []ErrorRule { return append([]ErrorRule(nil), builtinErrorRules...) }
func Filters() []Filter       { return append([]Filter(nil), builtinFilters...) }

func registerRules(rs ...Rule)     { builtinRules = append(builtinRules, rs...) }
func registerFilters(fs ...Filter) { builtinFilters = append(builtinFilters, fs...) }
