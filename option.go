package validator

import (
	"errors"
	"fmt"
	"maps"
	"reflect"
)

// Errors returned by invalid Validator configuration.
var (
	ErrInvalidOption    = errors.New("validator: invalid option")
	ErrInvalidSignature = errors.New("validator: invalid signature")
	ErrDuplicateRule    = errors.New("validator: duplicate rule")
	ErrDuplicateFilter  = errors.New("validator: duplicate filter")
)

// TagNameFunc derives a validation name; an empty result keeps the Go name.
type TagNameFunc func(field reflect.StructField) string

// TransformFunc transforms each resolved validation message.
type TransformFunc func(message string) string

// TranslatorFunc returns a rule template, or false to fall through.
type TranslatorFunc func(rule string) (string, bool)

type config struct {
	builtin                bool
	tagName                string
	tagNameFunc            TagNameFunc
	transformFunc          TransformFunc
	attributes             map[string]string
	messages               map[string]string
	translation            map[string]string
	translatorFn           TranslatorFunc
	strictRequired         bool
	privateFieldValidation bool
	parallel               int
	rules                  []Rule
	fallibleRules          []FallibleRule
	filters                []Filter
}

// Option configures an immutable Validator.
type Option func(*config) error

// WithTagName sets the validation struct tag. The default is "validate".
func WithTagName(name string) Option {
	return func(config *config) error {
		if name == "" {
			return fmt.Errorf("%w: tag name is empty", ErrInvalidOption)
		}
		config.tagName = name
		return nil
	}
}

// WithTagNameFunc derives validation field names.
func WithTagNameFunc(fn TagNameFunc) Option {
	return func(config *config) error {
		if fn == nil {
			return fmt.Errorf("%w: tag name function is nil", ErrInvalidOption)
		}
		config.tagNameFunc = fn
		return nil
	}
}

// WithTransformFunc transforms resolved messages.
func WithTransformFunc(fn TransformFunc) Option {
	return func(config *config) error {
		if fn == nil {
			return fmt.Errorf("%w: transform function is nil", ErrInvalidOption)
		}
		config.transformFunc = fn
		return nil
	}
}

// WithStrictRequired makes required reject zero values, not only absence.
func WithStrictRequired() Option {
	return func(config *config) error {
		config.strictRequired = true
		return nil
	}
}

// WithPrivateFieldValidation enables validation of unexported fields.
func WithPrivateFieldValidation() Option {
	return func(config *config) error {
		config.privateFieldValidation = true
		return nil
	}
}

// WithoutBuiltinRules constructs a Validator without built-in rules or filters.
func WithoutBuiltinRules() Option {
	return func(config *config) error {
		config.builtin = false
		return nil
	}
}

// WithAttributes sets display names used by {field}.
func WithAttributes(attributes map[string]string) Option {
	return func(config *config) error {
		if attributes == nil {
			return fmt.Errorf("%w: attributes are nil", ErrInvalidOption)
		}
		config.attributes = mergeStrMap(config.attributes, attributes)
		return nil
	}
}

// WithParallel enables bounded field concurrency when at least minFields are
// present. The option is disabled when omitted.
func WithParallel(minFields int) Option {
	return func(config *config) error {
		if minFields <= 0 {
			return fmt.Errorf("%w: parallel threshold must be positive", ErrInvalidOption)
		}
		config.parallel = minFields
		return nil
	}
}

// WithMessages sets templates keyed by "field.rule" or "rule".
func WithMessages(messages map[string]string) Option {
	return func(config *config) error {
		if messages == nil {
			return fmt.Errorf("%w: messages are nil", ErrInvalidOption)
		}
		config.messages = mergeStrMap(config.messages, messages)
		return nil
	}
}

// WithTranslation sets localized templates keyed by "field.rule" or "rule".
func WithTranslation(messages map[string]string) Option {
	return func(config *config) error {
		if messages == nil {
			return fmt.Errorf("%w: translation is nil", ErrInvalidOption)
		}
		config.translation = mergeStrMap(config.translation, messages)
		return nil
	}
}

// WithTranslator installs a dynamic template lookup.
func WithTranslator(fn TranslatorFunc) Option {
	return func(config *config) error {
		if fn == nil {
			return fmt.Errorf("%w: translator is nil", ErrInvalidOption)
		}
		config.translatorFn = fn
		return nil
	}
}

// WithRules adds custom boolean rules.
func WithRules(rules ...Rule) Option {
	return func(config *config) error {
		for i, rule := range rules {
			if nilLike(rule) {
				return fmt.Errorf("%w: rule %d is nil", ErrInvalidOption, i)
			}
			if err := validateSignature("rule", rule.Signature()); err != nil {
				return err
			}
		}
		config.rules = append(config.rules, rules...)
		return nil
	}
}

// WithFallibleRules adds rules whose backing operation can fail.
func WithFallibleRules(rules ...FallibleRule) Option {
	return func(config *config) error {
		for i, rule := range rules {
			if nilLike(rule) {
				return fmt.Errorf("%w: fallible rule %d is nil", ErrInvalidOption, i)
			}
			if err := validateSignature("rule", rule.Signature()); err != nil {
				return err
			}
		}
		config.fallibleRules = append(config.fallibleRules, rules...)
		return nil
	}
}

// WithFilters adds custom input filters.
func WithFilters(filters ...Filter) Option {
	return func(config *config) error {
		for i, filter := range filters {
			if nilLike(filter) {
				return fmt.Errorf("%w: filter %d is nil", ErrInvalidOption, i)
			}
			if err := validateSignature("filter", filter.Signature()); err != nil {
				return err
			}
		}
		config.filters = append(config.filters, filters...)
		return nil
	}
}

// WithRuleFunc adds a boolean rule backed by fn.
func WithRuleFunc(signature string, fn func(*Field) bool, message string) Option {
	return func(config *config) error {
		if fn == nil {
			return fmt.Errorf("%w: rule function is nil", ErrInvalidOption)
		}
		return WithRules(funcRule{sig: signature, fn: fn, msg: message})(config)
	}
}

// WithStringRule adds a string-oriented rule. Empty values pass and non-string
// values are rendered as strings before fn is called.
func WithStringRule(signature string, fn func(string, ...string) bool, message string) Option {
	return func(config *config) error {
		if fn == nil {
			return fmt.Errorf("%w: string rule function is nil", ErrInvalidOption)
		}
		return WithRuleFunc(signature, func(field *Field) bool {
			rv := field.Reflect()
			return isEmptyV(rv) || fn(valString(rv), field.Attrs()...)
		}, message)(config)
	}
}

func mergeStrMap(dst, src map[string]string) map[string]string {
	out := make(map[string]string, len(dst)+len(src))
	maps.Copy(out, dst)
	maps.Copy(out, src)
	return out
}

func nilLike(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}
