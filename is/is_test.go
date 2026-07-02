package is

import (
	"strings"
	"testing"
)

// TestIPMapped: IPv4-mapped IPv6 literals classify by textual form (IPv6, not IPv4).
func TestIPMapped(t *testing.T) {
	mapped := "::ffff:192.168.1.1"
	if IPv4(mapped) {
		t.Errorf("%q should not be IPv4", mapped)
	}
	if !IPv6(mapped) {
		t.Errorf("%q should be IPv6", mapped)
	}
	if !IPv4("192.168.1.1") {
		t.Error("192.168.1.1 should be IPv4")
	}
	if IPv6("192.168.1.1") {
		t.Error("192.168.1.1 should not be IPv6")
	}
	if !IPv6("2001:db8::1") {
		t.Error("2001:db8::1 should be IPv6")
	}
	if IPv4("2001:db8::1") {
		t.Error("2001:db8::1 should not be IPv4")
	}
}

func TestEmail(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"user@example.com", true},
		{"a.b-c+tag@sub.example.co.uk", true},
		{"", false},
		{"plainaddress", false},
		{"@no-local.com", false},
		{"no-domain@", false},
		{"two@@example.com", false},
		{"Name <a@b.com>", false}, // display-name form rejected
		{"a@b", false},            // regex requires a dotted domain
		// RFC 5321 length limits (beyond the bare regex):
		{strings.Repeat("a", 64) + "@e.com", true},          // local-part 64 = max
		{strings.Repeat("a", 65) + "@e.com", false},         // local-part > 64
		{"a@" + strings.Repeat("b", 256) + ".com", false},   // domain > 255
		{strings.Repeat("a", 250) + "@bbbbbbbb.com", false}, // address > 254
	}
	for _, c := range cases {
		if got := Email(c.in); got != c.want {
			t.Errorf("Email(%q)=%v want %v", c.in, got, c.want)
		}
	}
}

func TestURL(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"https://example.com", true},
		{"http://example.com/path?q=1#frag", true},
		{"ftp://host/file", true},
		{"", false},
		{"example.com", false},    // no scheme
		{"/relative/path", false}, // relative
		{"mailto:a@b.com", false}, // no host
		{"https://", false},       // empty host
	}
	for _, c := range cases {
		if got := URL(c.in); got != c.want {
			t.Errorf("URL(%q)=%v want %v", c.in, got, c.want)
		}
	}
}

func TestURI(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"https://example.com", true},
		{"mailto:a@b.com", true},
		{"urn:isbn:0451450523", true},
		{"", false},
		{"/relative/path", false}, // no scheme
		{"example.com", false},
	}
	for _, c := range cases {
		if got := URI(c.in); got != c.want {
			t.Errorf("URI(%q)=%v want %v", c.in, got, c.want)
		}
	}
}

func TestUUID(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"123e4567-e89b-12d3-a456-426614174000", true},
		{"00000000-0000-0000-0000-000000000000", true},
		{"123E4567-E89B-12D3-A456-426614174000", true}, // upper-case
		{"", false},
		{"123e4567e89b12d3a456426614174000", false},    // no dashes
		{"123e4567-e89b-12d3-a456-42661417400", false}, // too short
		{"123e4567-e89b-12d3-a456-426614174000-", false},
		{"ggggggg1-e89b-12d3-a456-426614174000", false}, // non-hex
	}
	for _, c := range cases {
		if got := UUID(c.in); got != c.want {
			t.Errorf("UUID(%q)=%v want %v", c.in, got, c.want)
		}
	}
}

func TestIP(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"127.0.0.1", true},
		{"::1", true},
		{"2001:db8::ff00:42:8329", true},
		{"", false},
		{"256.0.0.1", false},
		{"not-an-ip", false},
	}
	for _, c := range cases {
		if got := IP(c.in); got != c.want {
			t.Errorf("IP(%q)=%v want %v", c.in, got, c.want)
		}
	}
}

func TestIPv4(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"192.168.1.1", true},
		{"0.0.0.0", true},
		{"255.255.255.255", true},
		{"::1", false},
		{"2001:db8::1", false},
		{"256.1.1.1", false},
		{"", false},
	}
	for _, c := range cases {
		if got := IPv4(c.in); got != c.want {
			t.Errorf("IPv4(%q)=%v want %v", c.in, got, c.want)
		}
	}
}

func TestIPv6(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"::1", true},
		{"2001:db8::ff00:42:8329", true},
		{"fe80::1", true},
		{"127.0.0.1", false}, // v4 is not v6
		{"192.168.1.1", false},
		{"", false},
		{"gggg::1", false},
	}
	for _, c := range cases {
		if got := IPv6(c.in); got != c.want {
			t.Errorf("IPv6(%q)=%v want %v", c.in, got, c.want)
		}
	}
}

func TestJSON(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{`{"a":1}`, true},
		{`[1,2,3]`, true},
		{`"string"`, true},
		{`123`, true},
		{`true`, true},
		{`null`, true},
		{"", false},
		{`{"a":}`, false},
		{`{a:1}`, false},
		{`{`, false},
	}
	for _, c := range cases {
		if got := JSON(c.in); got != c.want {
			t.Errorf("JSON(%q)=%v want %v", c.in, got, c.want)
		}
	}
}

func TestBase64(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"aGVsbG8=", true},
		{"Zm9vYmFy", true},
		{"YQ==", true},
		{"YWI=", true},
		{"", false},
		{"not base64!!", false},
		{"aGVsbG8", false}, // bad padding
		{"====", false},
	}
	for _, c := range cases {
		if got := Base64(c.in); got != c.want {
			t.Errorf("Base64(%q)=%v want %v", c.in, got, c.want)
		}
	}
}

func TestMAC(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"00:1A:2B:3C:4D:5E", true},
		{"00-1A-2B-3C-4D-5E", true},
		{"001A.2B3C.4D5E", true},
		{"", false},
		{"00:1A:2B:3C:4D", false},
		{"not-a-mac", false},
		{"ZZ:1A:2B:3C:4D:5E", false},
	}
	for _, c := range cases {
		if got := MAC(c.in); got != c.want {
			t.Errorf("MAC(%q)=%v want %v", c.in, got, c.want)
		}
	}
}

func TestHostname(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"example.com", true},
		{"sub.example.co.uk", true},
		{"localhost", true},
		{"a", true},
		{"example.com.", true}, // trailing root dot allowed
		{"123.example.com", true},
		{"", false},
		{"-bad.example.com", false}, // leading hyphen
		{"bad-.example.com", false}, // trailing hyphen
		{"exa mple.com", false},     // space
		{"under_score.com", false},  // underscore
		{".", false},
		{"a..b", false}, // empty label
	}
	for _, c := range cases {
		if got := Hostname(c.in); got != c.want {
			t.Errorf("Hostname(%q)=%v want %v", c.in, got, c.want)
		}
	}
}
