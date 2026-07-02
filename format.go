package validator

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/libtnb/validator/conv"
	"github.com/libtnb/validator/is"
)

func init() {
	registerRules(
		&emailRule{},
		&urlRule{},
		&uriRule{},
		&uuidRule{},
		&ipRule{},
		&ipv4Rule{},
		&ipv6Rule{},
		&jsonRule{},
		&base64Rule{},
		&macRule{},
		&hostnameRule{},
		&datetimeRule{},
		&dateRule{},
		&regexRule{},
		&notRegexRule{},
	)
}

var (
	_ Rule = (*emailRule)(nil)
	_ Rule = (*urlRule)(nil)
	_ Rule = (*uriRule)(nil)
	_ Rule = (*uuidRule)(nil)
	_ Rule = (*ipRule)(nil)
	_ Rule = (*ipv4Rule)(nil)
	_ Rule = (*ipv6Rule)(nil)
	_ Rule = (*jsonRule)(nil)
	_ Rule = (*base64Rule)(nil)
	_ Rule = (*macRule)(nil)
	_ Rule = (*hostnameRule)(nil)
	_ Rule = (*datetimeRule)(nil)
	_ Rule = (*dateRule)(nil)
	_ Rule = (*regexRule)(nil)
	_ Rule = (*notRegexRule)(nil)
)

// emailRule validates an email address.
type emailRule struct{}

func (r *emailRule) Signature() string { return "email" }

func (r *emailRule) Passes(f Field) bool { return stringFormat(f, is.Email) }

func (r *emailRule) Message() string { return "The {field} must be a valid email address." }

// urlRule validates an absolute URL (scheme and host).
type urlRule struct{}

func (r *urlRule) Signature() string { return "url" }

func (r *urlRule) Passes(f Field) bool { return stringFormat(f, is.URL) }

func (r *urlRule) Message() string { return "The {field} must be a valid URL." }

// uriRule validates an absolute URI (scheme required).
type uriRule struct{}

func (r *uriRule) Signature() string { return "uri" }

func (r *uriRule) Passes(f Field) bool { return stringFormat(f, is.URI) }

func (r *uriRule) Message() string { return "The {field} must be a valid URI." }

// uuidRule validates a canonical UUID.
type uuidRule struct{}

func (r *uuidRule) Signature() string { return "uuid" }

func (r *uuidRule) Passes(f Field) bool { return stringFormat(f, is.UUID) }

func (r *uuidRule) Message() string { return "The {field} must be a valid UUID." }

// ipRule validates an IPv4 or IPv6 address.
type ipRule struct{}

func (r *ipRule) Signature() string { return "ip" }

func (r *ipRule) Passes(f Field) bool { return stringFormat(f, is.IP) }

func (r *ipRule) Message() string { return "The {field} must be a valid IP address." }

// ipv4Rule validates an IPv4 address.
type ipv4Rule struct{}

func (r *ipv4Rule) Signature() string { return "ipv4" }

func (r *ipv4Rule) Passes(f Field) bool { return stringFormat(f, is.IPv4) }

func (r *ipv4Rule) Message() string { return "The {field} must be a valid IPv4 address." }

// ipv6Rule validates an IPv6 address.
type ipv6Rule struct{}

func (r *ipv6Rule) Signature() string { return "ipv6" }

func (r *ipv6Rule) Passes(f Field) bool { return stringFormat(f, is.IPv6) }

func (r *ipv6Rule) Message() string { return "The {field} must be a valid IPv6 address." }

// jsonRule validates well-formed JSON.
type jsonRule struct{}

func (r *jsonRule) Signature() string { return "json" }

func (r *jsonRule) Passes(f Field) bool { return stringFormat(f, is.JSON) }

func (r *jsonRule) Message() string { return "The {field} must be a valid JSON string." }

// base64Rule validates standard base64 text.
type base64Rule struct{}

func (r *base64Rule) Signature() string { return "base64" }

func (r *base64Rule) Passes(f Field) bool { return stringFormat(f, is.Base64) }

func (r *base64Rule) Message() string { return "The {field} must be a valid base64 string." }

// macRule validates a MAC address.
type macRule struct{}

func (r *macRule) Signature() string { return "mac" }

func (r *macRule) Passes(f Field) bool { return stringFormat(f, is.MAC) }

func (r *macRule) Message() string { return "The {field} must be a valid MAC address." }

// hostnameRule validates an RFC 1123 DNS hostname.
type hostnameRule struct{}

func (r *hostnameRule) Signature() string { return "hostname" }

func (r *hostnameRule) Passes(f Field) bool { return stringFormat(f, is.Hostname) }

func (r *hostnameRule) Message() string { return "The {field} must be a valid hostname." }

// datetimeRule parses against the layout arg (datetime:2006-01-02) or default layouts.
type datetimeRule struct{}

func (r *datetimeRule) Signature() string { return "datetime" }

func (r *datetimeRule) Passes(f Field) bool {
	var layouts []string
	if attrs := f.Attrs(); len(attrs) > 0 && attrs[0] != "" {
		layouts = []string{attrs[0]}
	}
	return parsesAsTime(f, layouts...)
}

// Message avoids {0}: the optional layout arg must not leave a dangling placeholder.
func (r *datetimeRule) Message() string {
	return "The {field} must be a valid date-time."
}

// dateRule parses a date/time using default layouts.
type dateRule struct{}

func (r *dateRule) Signature() string { return "date" }

func (r *dateRule) Passes(f Field) bool { return parsesAsTime(f) }

func (r *dateRule) Message() string { return "The {field} must be a valid date." }

// regexRule matches the pattern arg (regex:^[a-z]+$).
type regexRule struct{}

func (r *regexRule) Signature() string { return "regex" }

func (r *regexRule) Passes(f Field) bool { return regexMatch(f, true) }

// CheckArgs makes an invalid pattern a compile-time config error, not a runtime data error.
func (r *regexRule) CheckArgs(args []string) error { return checkRegexArg(args) }

func (r *regexRule) Message() string { return "The {field} format is invalid." }

// notRegexRule passes when the value does NOT match the pattern arg.
type notRegexRule struct{}

func (r *notRegexRule) Signature() string { return "not_regex" }

func (r *notRegexRule) Passes(f Field) bool { return regexMatch(f, false) }

func (r *notRegexRule) CheckArgs(args []string) error { return checkRegexArg(args) }

func (r *notRegexRule) Message() string { return "The {field} format is invalid." }

// compilePasses (leafCompiler): arg-less rules hand the compiler their Passes
// method directly, skipping per-eval arg binding and interface dispatch.
var (
	_ leafCompiler = (*emailRule)(nil)
	_ leafCompiler = (*urlRule)(nil)
	_ leafCompiler = (*uriRule)(nil)
	_ leafCompiler = (*uuidRule)(nil)
	_ leafCompiler = (*ipRule)(nil)
	_ leafCompiler = (*ipv4Rule)(nil)
	_ leafCompiler = (*ipv6Rule)(nil)
	_ leafCompiler = (*jsonRule)(nil)
	_ leafCompiler = (*base64Rule)(nil)
	_ leafCompiler = (*macRule)(nil)
	_ leafCompiler = (*hostnameRule)(nil)
	_ leafCompiler = (*dateRule)(nil)
)

func (r *emailRule) compilePasses([]string) func(Field) bool    { return r.Passes }
func (r *urlRule) compilePasses([]string) func(Field) bool      { return r.Passes }
func (r *uriRule) compilePasses([]string) func(Field) bool      { return r.Passes }
func (r *uuidRule) compilePasses([]string) func(Field) bool     { return r.Passes }
func (r *ipRule) compilePasses([]string) func(Field) bool       { return r.Passes }
func (r *ipv4Rule) compilePasses([]string) func(Field) bool     { return r.Passes }
func (r *ipv6Rule) compilePasses([]string) func(Field) bool     { return r.Passes }
func (r *jsonRule) compilePasses([]string) func(Field) bool     { return r.Passes }
func (r *base64Rule) compilePasses([]string) func(Field) bool   { return r.Passes }
func (r *macRule) compilePasses([]string) func(Field) bool      { return r.Passes }
func (r *hostnameRule) compilePasses([]string) func(Field) bool { return r.Passes }
func (r *dateRule) compilePasses([]string) func(Field) bool     { return r.Passes }

func stringFormat(f Field, ok func(string) bool) bool {
	if isEmptyV(f.Val()) {
		return true
	}
	return ok(valString(f.Val()))
}

func parsesAsTime(f Field, layouts ...string) bool {
	rv := f.Val()
	if isEmptyV(rv) {
		return true
	}
	if rv.Type() == timeType {
		return !rv.Interface().(time.Time).IsZero() // layout only constrains strings
	}
	s := strings.TrimSpace(valString(rv))
	if s == "" {
		return false
	}
	if len(layouts) == 0 {
		layouts = conv.DefaultTimeLayouts
	}
	for _, l := range layouts {
		if _, err := time.Parse(l, s); err == nil {
			return true
		}
	}
	return false
}

// regexMatch fails closed on a missing or invalid pattern.
func regexMatch(f Field, want bool) bool {
	if isEmptyV(f.Val()) {
		return true
	}
	attrs := f.Attrs()
	if len(attrs) == 0 {
		return false
	}
	re, err := compileRegex(attrs[0])
	if err != nil {
		return false
	}
	return re.MatchString(valString(f.Val())) == want
}

func checkRegexArg(args []string) error {
	if len(args) == 0 || args[0] == "" {
		return errors.New("a pattern argument is required")
	}
	if _, err := regexp.Compile(args[0]); err != nil {
		return fmt.Errorf("invalid pattern: %v", err)
	}
	return nil
}
