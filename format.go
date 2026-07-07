package validator

import (
	"errors"
	"fmt"
	"math"
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
		&fqdnRule{},
		&datetimeRule{},
		&dateRule{},
		&regexRule{},
		&notRegexRule{},
		&e164Rule{},
		&semverRule{},
		&ulidRule{},
		&jwtRule{},
		&hexColorRule{},
		&latitudeRule{},
		&longitudeRule{},
		&timezoneRule{},
		&luhnRule{},
		&creditCardRule{},
		&portRule{},
		&cidrRule{}, &cidrv4Rule{}, &cidrv6Rule{},
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
	_ Rule = (*fqdnRule)(nil)
	_ Rule = (*datetimeRule)(nil)
	_ Rule = (*dateRule)(nil)
	_ Rule = (*regexRule)(nil)
	_ Rule = (*notRegexRule)(nil)
	_ Rule = (*e164Rule)(nil)
	_ Rule = (*semverRule)(nil)
	_ Rule = (*ulidRule)(nil)
	_ Rule = (*jwtRule)(nil)
	_ Rule = (*hexColorRule)(nil)
	_ Rule = (*latitudeRule)(nil)
	_ Rule = (*longitudeRule)(nil)
	_ Rule = (*timezoneRule)(nil)
	_ Rule = (*luhnRule)(nil)
	_ Rule = (*creditCardRule)(nil)
	_ Rule = (*portRule)(nil)
	_ Rule = (*cidrRule)(nil)
	_ Rule = (*cidrv4Rule)(nil)
	_ Rule = (*cidrv6Rule)(nil)
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

// fqdnRule validates a fully qualified domain name (TLD required).
type fqdnRule struct{}

func (r *fqdnRule) Signature() string { return "fqdn" }

func (r *fqdnRule) Passes(f Field) bool { return stringFormat(f, is.FQDN) }

func (r *fqdnRule) Message() string {
	return "The {field} must be a valid fully qualified domain name."
}

// e164Rule validates an E.164 international phone number.
type e164Rule struct{}

func (r *e164Rule) Signature() string { return "e164" }

func (r *e164Rule) Passes(f Field) bool { return stringFormat(f, is.E164) }

func (r *e164Rule) Message() string { return "The {field} must be a valid E.164 phone number." }

// semverRule validates a semantic version.
type semverRule struct{}

func (r *semverRule) Signature() string { return "semver" }

func (r *semverRule) Passes(f Field) bool { return stringFormat(f, is.Semver) }

func (r *semverRule) Message() string { return "The {field} must be a valid semantic version." }

// ulidRule validates a ULID.
type ulidRule struct{}

func (r *ulidRule) Signature() string { return "ulid" }

func (r *ulidRule) Passes(f Field) bool { return stringFormat(f, is.ULID) }

func (r *ulidRule) Message() string { return "The {field} must be a valid ULID." }

// jwtRule validates a JWT's three-segment shape.
type jwtRule struct{}

func (r *jwtRule) Signature() string { return "jwt" }

func (r *jwtRule) Passes(f Field) bool { return stringFormat(f, is.JWT) }

func (r *jwtRule) Message() string { return "The {field} must be a valid JWT." }

// hexColorRule validates a #-prefixed hex color.
type hexColorRule struct{}

func (r *hexColorRule) Signature() string { return "hexcolor" }

func (r *hexColorRule) Passes(f Field) bool { return stringFormat(f, is.HexColor) }

func (r *hexColorRule) Message() string { return "The {field} must be a valid hex color." }

// latitudeRule validates a decimal latitude; numeric values render canonically first.
type latitudeRule struct{}

func (r *latitudeRule) Signature() string { return "latitude" }

func (r *latitudeRule) Passes(f Field) bool { return stringFormat(f, is.Latitude) }

func (r *latitudeRule) Message() string { return "The {field} must be a valid latitude." }

// longitudeRule validates a decimal longitude; numeric values render canonically first.
type longitudeRule struct{}

func (r *longitudeRule) Signature() string { return "longitude" }

func (r *longitudeRule) Passes(f Field) bool { return stringFormat(f, is.Longitude) }

func (r *longitudeRule) Message() string { return "The {field} must be a valid longitude." }

// timezoneRule validates an IANA time zone name.
type timezoneRule struct{}

func (r *timezoneRule) Signature() string { return "timezone" }

func (r *timezoneRule) Passes(f Field) bool { return stringFormat(f, is.TimeZone) }

func (r *timezoneRule) Message() string { return "The {field} must be a valid time zone." }

// luhnRule validates a digit string against the Luhn checksum.
type luhnRule struct{}

func (r *luhnRule) Signature() string { return "luhn" }

func (r *luhnRule) Passes(f Field) bool { return stringFormat(f, is.Luhn) }

func (r *luhnRule) Message() string { return "The {field} must pass the Luhn check." }

// creditCardRule validates a credit card number.
type creditCardRule struct{}

func (r *creditCardRule) Signature() string { return "credit_card" }

func (r *creditCardRule) Passes(f Field) bool { return stringFormat(f, is.CreditCard) }

func (r *creditCardRule) Message() string { return "The {field} must be a valid credit card number." }

// portRule validates an integer port in [1, 65535].
type portRule struct{}

func (r *portRule) Signature() string { return "port" }

func (r *portRule) Passes(f Field) bool {
	rv := f.Val()
	if isEmptyV(rv) {
		return true
	}
	n, ok := numericValue(rv)
	return ok && n == math.Trunc(n) && n >= 1 && n <= 65535
}

func (r *portRule) Message() string { return "The {field} must be a valid port number." }

// cidrRule validates CIDR notation (IPv4 or IPv6).
type cidrRule struct{}

func (r *cidrRule) Signature() string { return "cidr" }

func (r *cidrRule) Passes(f Field) bool { return stringFormat(f, is.CIDR) }

func (r *cidrRule) Message() string { return "The {field} must be a valid CIDR notation." }

// cidrv4Rule validates IPv4 CIDR notation.
type cidrv4Rule struct{}

func (r *cidrv4Rule) Signature() string { return "cidrv4" }

func (r *cidrv4Rule) Passes(f Field) bool { return stringFormat(f, is.CIDRv4) }

func (r *cidrv4Rule) Message() string { return "The {field} must be a valid IPv4 CIDR notation." }

// cidrv6Rule validates IPv6 CIDR notation.
type cidrv6Rule struct{}

func (r *cidrv6Rule) Signature() string { return "cidrv6" }

func (r *cidrv6Rule) Passes(f Field) bool { return stringFormat(f, is.CIDRv6) }

func (r *cidrv6Rule) Message() string { return "The {field} must be a valid IPv6 CIDR notation." }

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
	_ leafCompiler = (*fqdnRule)(nil)
	_ leafCompiler = (*dateRule)(nil)
	_ leafCompiler = (*e164Rule)(nil)
	_ leafCompiler = (*semverRule)(nil)
	_ leafCompiler = (*ulidRule)(nil)
	_ leafCompiler = (*jwtRule)(nil)
	_ leafCompiler = (*hexColorRule)(nil)
	_ leafCompiler = (*latitudeRule)(nil)
	_ leafCompiler = (*longitudeRule)(nil)
	_ leafCompiler = (*timezoneRule)(nil)
	_ leafCompiler = (*luhnRule)(nil)
	_ leafCompiler = (*creditCardRule)(nil)
	_ leafCompiler = (*portRule)(nil)
	_ leafCompiler = (*cidrRule)(nil)
	_ leafCompiler = (*cidrv4Rule)(nil)
	_ leafCompiler = (*cidrv6Rule)(nil)
)

func (r *emailRule) compilePasses([]string) func(Field) bool      { return r.Passes }
func (r *urlRule) compilePasses([]string) func(Field) bool        { return r.Passes }
func (r *uriRule) compilePasses([]string) func(Field) bool        { return r.Passes }
func (r *uuidRule) compilePasses([]string) func(Field) bool       { return r.Passes }
func (r *ipRule) compilePasses([]string) func(Field) bool         { return r.Passes }
func (r *ipv4Rule) compilePasses([]string) func(Field) bool       { return r.Passes }
func (r *ipv6Rule) compilePasses([]string) func(Field) bool       { return r.Passes }
func (r *jsonRule) compilePasses([]string) func(Field) bool       { return r.Passes }
func (r *base64Rule) compilePasses([]string) func(Field) bool     { return r.Passes }
func (r *macRule) compilePasses([]string) func(Field) bool        { return r.Passes }
func (r *hostnameRule) compilePasses([]string) func(Field) bool   { return r.Passes }
func (r *fqdnRule) compilePasses([]string) func(Field) bool       { return r.Passes }
func (r *dateRule) compilePasses([]string) func(Field) bool       { return r.Passes }
func (r *e164Rule) compilePasses([]string) func(Field) bool       { return r.Passes }
func (r *semverRule) compilePasses([]string) func(Field) bool     { return r.Passes }
func (r *ulidRule) compilePasses([]string) func(Field) bool       { return r.Passes }
func (r *jwtRule) compilePasses([]string) func(Field) bool        { return r.Passes }
func (r *hexColorRule) compilePasses([]string) func(Field) bool   { return r.Passes }
func (r *latitudeRule) compilePasses([]string) func(Field) bool   { return r.Passes }
func (r *longitudeRule) compilePasses([]string) func(Field) bool  { return r.Passes }
func (r *timezoneRule) compilePasses([]string) func(Field) bool   { return r.Passes }
func (r *luhnRule) compilePasses([]string) func(Field) bool       { return r.Passes }
func (r *creditCardRule) compilePasses([]string) func(Field) bool { return r.Passes }
func (r *portRule) compilePasses([]string) func(Field) bool       { return r.Passes }
func (r *cidrRule) compilePasses([]string) func(Field) bool       { return r.Passes }
func (r *cidrv4Rule) compilePasses([]string) func(Field) bool     { return r.Passes }
func (r *cidrv6Rule) compilePasses([]string) func(Field) bool     { return r.Passes }

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
