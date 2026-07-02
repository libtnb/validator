package validator

import (
	"sync"
)

type registry struct {
	mu       sync.RWMutex
	rules    map[string]Rule
	errRules map[string]ErrorRule
	filters  map[string]Filter
}

func newRegistry() *registry {
	return &registry{
		rules:    make(map[string]Rule),
		errRules: make(map[string]ErrorRule),
		filters:  make(map[string]Filter),
	}
}

func (r *registry) addRule(rule Rule) {
	r.mu.Lock()
	r.rules[rule.Signature()] = rule
	r.mu.Unlock()
}

func (r *registry) addErrorRule(rule ErrorRule) {
	r.mu.Lock()
	r.errRules[rule.Signature()] = rule
	r.mu.Unlock()
}

func (r *registry) addFilter(f Filter) {
	r.mu.Lock()
	r.filters[f.Signature()] = f
	r.mu.Unlock()
}

func (r *registry) rule(sig string) (Rule, bool) {
	r.mu.RLock()
	rule, ok := r.rules[sig]
	r.mu.RUnlock()
	return rule, ok
}

func (r *registry) errorRule(sig string) (ErrorRule, bool) {
	r.mu.RLock()
	rule, ok := r.errRules[sig]
	r.mu.RUnlock()
	return rule, ok
}

func (r *registry) filter(sig string) (Filter, bool) {
	r.mu.RLock()
	f, ok := r.filters[sig]
	r.mu.RUnlock()
	return f, ok
}
