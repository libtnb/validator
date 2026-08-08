package validator

import (
	"fmt"
)

type registry struct {
	rules    map[string]Rule
	fallible map[string]FallibleRule
	filters  map[string]Filter
}

func (r *registry) addRule(rule Rule) error {
	signature := rule.Signature()
	if _, exists := r.rules[signature]; exists {
		return fmt.Errorf("%w: %q", ErrDuplicateRule, signature)
	}
	if _, exists := r.fallible[signature]; exists {
		return fmt.Errorf("%w: %q", ErrDuplicateRule, signature)
	}
	r.rules[signature] = rule
	return nil
}

func (r *registry) addFallibleRule(rule FallibleRule) error {
	signature := rule.Signature()
	if _, exists := r.rules[signature]; exists {
		return fmt.Errorf("%w: %q", ErrDuplicateRule, signature)
	}
	if _, exists := r.fallible[signature]; exists {
		return fmt.Errorf("%w: %q", ErrDuplicateRule, signature)
	}
	r.fallible[signature] = rule
	return nil
}

func (r *registry) addFilter(f Filter) error {
	signature := f.Signature()
	if _, exists := r.filters[signature]; exists {
		return fmt.Errorf("%w: %q", ErrDuplicateFilter, signature)
	}
	r.filters[signature] = f
	return nil
}

func (r *registry) rule(sig string) (Rule, bool) {
	rule, ok := r.rules[sig]
	return rule, ok
}

func (r *registry) fallibleRule(sig string) (FallibleRule, bool) {
	rule, ok := r.fallible[sig]
	return rule, ok
}

func (r *registry) filter(sig string) (Filter, bool) {
	f, ok := r.filters[sig]
	return f, ok
}

func newRegistry() *registry {
	return &registry{
		rules:    make(map[string]Rule),
		fallible: make(map[string]FallibleRule),
		filters:  make(map[string]Filter),
	}
}
