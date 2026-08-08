package validator

import (
	"errors"
	"fmt"
)

// Errors that distinguish input, rule, and runtime failures.
var (
	ErrNilContext     = errors.New("validator: nil context")
	ErrInvalidInput   = errors.New("validator: invalid input")
	ErrInvalidRules   = errors.New("validator: invalid rules")
	ErrRuleEvaluation = errors.New("validator: rule evaluation failed")
	ErrRulePanic      = errors.New("validator: rule panicked")
)

// RuleError identifies a rule whose evaluation could not be completed.
type RuleError struct {
	Field string
	Rule  string
	Err   error
}

func (e *RuleError) Error() string {
	if e == nil {
		return ErrRuleEvaluation.Error()
	}
	if e.Err == nil {
		switch {
		case e.Field != "" && e.Rule != "":
			return fmt.Sprintf("validator: field %q rule %q: rule evaluation failed", e.Field, e.Rule)
		case e.Field != "":
			return fmt.Sprintf("validator: field %q: rule evaluation failed", e.Field)
		default:
			return ErrRuleEvaluation.Error()
		}
	}
	switch {
	case e.Field != "" && e.Rule != "":
		return fmt.Sprintf("validator: field %q rule %q: %v", e.Field, e.Rule, e.Err)
	case e.Field != "":
		return fmt.Sprintf("validator: field %q: %v", e.Field, e.Err)
	default:
		return "validator: " + e.Err.Error()
	}
}

func (e *RuleError) Unwrap() error {
	if e == nil || e.Err == nil {
		return ErrRuleEvaluation
	}
	return errors.Join(ErrRuleEvaluation, e.Err)
}

// RulesError identifies an invalid rule expression.
type RulesError struct {
	Field string
	Err   error
}

func (e *RulesError) Error() string {
	if e == nil {
		return ErrInvalidRules.Error()
	}
	if e.Err == nil {
		if e.Field == "" {
			return ErrInvalidRules.Error()
		}
		return fmt.Sprintf("validator: field %q: invalid rules", e.Field)
	}
	if e.Field == "" {
		return "validator: invalid rules: " + e.Err.Error()
	}
	return fmt.Sprintf("validator: field %q: %v", e.Field, e.Err)
}

func (e *RulesError) Unwrap() error {
	if e == nil || e.Err == nil {
		return ErrInvalidRules
	}
	return errors.Join(ErrInvalidRules, e.Err)
}
