// Package is provides pure predicates for common string formats.
package is

import (
	"encoding/json"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var (
	reUUID     = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	reBase64   = regexp.MustCompile(`^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=|[A-Za-z0-9+/]{4})$`)
	reHostname = regexp.MustCompile(`^(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`)
	reE164     = regexp.MustCompile(`^\+[1-9]\d{1,14}$`)
	// reSemver is the official semver.org suggested pattern.
	reSemver   = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$`)
	reULID     = regexp.MustCompile(`(?i)^[0-9A-HJKMNP-TV-Z]{26}$`)
	reJWT      = regexp.MustCompile(`^[A-Za-z0-9-_]+\.[A-Za-z0-9-_]+\.[A-Za-z0-9-_]*$`)
	reHexColor = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3,4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)
	// reFQDN uses the same strict RFC 1123 label rule as reHostname (labels never
	// start or end with '-') plus a mandatory letter-led TLD.
	reFQDN  = regexp.MustCompile(`^(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)
	reLat   = regexp.MustCompile(`^[-+]?([1-8]?\d(\.\d+)?|90(\.0+)?)$`)
	reLong  = regexp.MustCompile(`^[-+]?(180(\.0+)?|((1[0-7]\d)|([1-9]?\d))(\.\d+)?)$`)
	reEmail = regexp.MustCompile("^(?:(?:(?:(?:[a-zA-Z]|\\d|[!#\\$%&'\\*\\+\\-\\/=\\?\\^_`{\\|}~]|[\\x{00A0}-\\x{D7FF}\\x{F900}-\\x{FDCF}\\x{FDF0}-\\x{FFEF}])+(?:\\.([a-zA-Z]|\\d|[!#\\$%&'\\*\\+\\-\\/=\\?\\^_`{\\|}~]|[\\x{00A0}-\\x{D7FF}\\x{F900}-\\x{FDCF}\\x{FDF0}-\\x{FFEF}])+)*)|(?:(?:\\x22)(?:(?:(?:(?:\\x20|\\x09)*(?:\\x0d\\x0a))?(?:\\x20|\\x09)+)?(?:(?:[\\x01-\\x08\\x0b\\x0c\\x0e-\\x1f\\x7f]|\\x21|[\\x23-\\x5b]|[\\x5d-\\x7e]|[\\x{00A0}-\\x{D7FF}\\x{F900}-\\x{FDCF}\\x{FDF0}-\\x{FFEF}])|(?:(?:[\\x01-\\x09\\x0b\\x0c\\x0d-\\x7f]|[\\x{00A0}-\\x{D7FF}\\x{F900}-\\x{FDCF}\\x{FDF0}-\\x{FFEF}]))))*(?:(?:(?:\\x20|\\x09)*(?:\\x0d\\x0a))?(\\x20|\\x09)+)?(?:\\x22))))@(?:(?:(?:[a-zA-Z]|\\d|[\\x{00A0}-\\x{D7FF}\\x{F900}-\\x{FDCF}\\x{FDF0}-\\x{FFEF}])|(?:(?:[a-zA-Z]|\\d|[\\x{00A0}-\\x{D7FF}\\x{F900}-\\x{FDCF}\\x{FDF0}-\\x{FFEF}])(?:[a-zA-Z]|\\d|-|\\.|~|[\\x{00A0}-\\x{D7FF}\\x{F900}-\\x{FDCF}\\x{FDF0}-\\x{FFEF}])*(?:[a-zA-Z]|\\d|[\\x{00A0}-\\x{D7FF}\\x{F900}-\\x{FDCF}\\x{FDF0}-\\x{FFEF}])))\\.)+(?:(?:[a-zA-Z]|[\\x{00A0}-\\x{D7FF}\\x{F900}-\\x{FDCF}\\x{FDF0}-\\x{FFEF}])|(?:(?:[a-zA-Z]|[\\x{00A0}-\\x{D7FF}\\x{F900}-\\x{FDCF}\\x{FDF0}-\\x{FFEF}])(?:[a-zA-Z]|\\d|-|\\.|~|[\\x{00A0}-\\x{D7FF}\\x{F900}-\\x{FDCF}\\x{FDF0}-\\x{FFEF}])*(?:[a-zA-Z]|[\\x{00A0}-\\x{D7FF}\\x{F900}-\\x{FDCF}\\x{FDF0}-\\x{FFEF}])))\\.?$")
)

func Email(s string) bool {
	if s == "" || len(s) > 254 || !reEmail.MatchString(s) {
		return false
	}
	at := strings.LastIndexByte(s, '@')
	return at <= 64 && len(s)-at-1 <= 255
}

func URL(s string) bool {
	if s == "" {
		return false
	}
	u, err := url.ParseRequestURI(s)
	if err != nil {
		return false
	}
	return u.Scheme != "" && u.Host != ""
}

// URI requires a scheme but no host, unlike URL.
func URI(s string) bool {
	if s == "" {
		return false
	}
	u, err := url.ParseRequestURI(s)
	if err != nil {
		return false
	}
	return u.Scheme != ""
}

func UUID(s string) bool {
	return reUUID.MatchString(s)
}

func IP(s string) bool {
	return net.ParseIP(s) != nil
}

func IPv4(s string) bool {
	return !strings.Contains(s, ":") && net.ParseIP(s) != nil
}

func IPv6(s string) bool {
	return strings.Contains(s, ":") && net.ParseIP(s) != nil
}

// JSON reports whether s is well-formed JSON (empty is not).
func JSON(s string) bool {
	if s == "" {
		return false
	}
	return json.Valid([]byte(s))
}

func Base64(s string) bool {
	if s == "" {
		return false
	}
	return reBase64.MatchString(s)
}

// MAC accepts any net.ParseMAC format.
func MAC(s string) bool {
	if s == "" {
		return false
	}
	_, err := net.ParseMAC(s)
	return err == nil
}

func Hostname(s string) bool {
	if s == "" || len(s) > 253 {
		return false
	}
	// Trailing dot (root label) is valid in DNS names.
	host := strings.TrimSuffix(s, ".")
	if host == "" {
		return false
	}
	return reHostname.MatchString(host)
}

// E164 reports an international phone number in E.164 form (+14155552671).
func E164(s string) bool {
	return reE164.MatchString(s)
}

// Semver reports a semantic version per semver.org (no leading "v").
func Semver(s string) bool {
	return reSemver.MatchString(s)
}

// ULID reports a Crockford base32 ULID (case-insensitive).
func ULID(s string) bool {
	return reULID.MatchString(s)
}

// JWT reports the three dot-separated base64url segments of a JWT.
func JWT(s string) bool {
	return reJWT.MatchString(s)
}

// HexColor reports a #-prefixed hex color (3, 4, 6 or 8 digits).
func HexColor(s string) bool {
	return reHexColor.MatchString(s)
}

// FQDN reports a fully qualified domain name (requires a TLD, unlike Hostname).
func FQDN(s string) bool {
	if s == "" || len(s) > 253 {
		return false
	}
	// Trailing dot (root label) is valid, as in Hostname.
	host := strings.TrimSuffix(s, ".")
	if host == "" {
		return false
	}
	return reFQDN.MatchString(host)
}

// Latitude reports a decimal latitude in [-90, 90].
func Latitude(s string) bool {
	return reLat.MatchString(s)
}

// Longitude reports a decimal longitude in [-180, 180].
func Longitude(s string) bool {
	return reLong.MatchString(s)
}

// TimeZone reports an IANA time zone name ("UTC", "Asia/Shanghai");
// "Local" and the empty name are rejected as environment-dependent.
func TimeZone(s string) bool {
	if s == "" || strings.EqualFold(s, "local") {
		return false
	}
	_, err := time.LoadLocation(s)
	return err == nil
}

// Luhn reports a digit string passing the Luhn checksum.
func Luhn(s string) bool {
	if s == "" {
		return false
	}
	sum, double := 0, false
	for i := len(s) - 1; i >= 0; i-- {
		c := s[i]
		if c < '0' || c > '9' {
			return false
		}
		d := int(c - '0')
		if double {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}
	return sum%10 == 0
}

// CreditCard reports a 12-19 digit card number passing Luhn; spaces and dashes
// between digit groups are ignored.
func CreditCard(s string) bool {
	if s == "" {
		return false
	}
	digits := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c >= '0' && c <= '9':
			digits = append(digits, c)
		case c == ' ' || c == '-':
			// group separator
		default:
			return false
		}
	}
	if len(digits) < 12 || len(digits) > 19 {
		return false
	}
	return Luhn(string(digits))
}

// CIDR reports a valid CIDR notation (IPv4 or IPv6).
func CIDR(s string) bool {
	_, _, err := net.ParseCIDR(s)
	return err == nil
}

// CIDRv4 reports a valid IPv4 CIDR notation.
func CIDRv4(s string) bool {
	return !strings.Contains(s, ":") && CIDR(s)
}

// CIDRv6 reports a valid IPv6 CIDR notation.
func CIDRv6(s string) bool {
	return strings.Contains(s, ":") && CIDR(s)
}
