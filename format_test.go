package validator

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

type formatCase struct {
	val   any
	attrs []string
	want  bool
}

func runFormat(t *testing.T, sig string, r Rule, cases []formatCase) {
	t.Helper()
	for _, c := range cases {
		got := r.Passes(fakeField{val: reflect.ValueOf(c.val), attrs: c.attrs, name: "x"})
		if got != c.want {
			t.Errorf("%s.Passes(val=%v attrs=%v)=%v want %v", sig, c.val, c.attrs, got, c.want)
		}
	}
}

func TestEmailRule(t *testing.T) {
	r := &emailRule{}
	if r.Signature() != "email" {
		t.Fatalf("Signature()=%q", r.Signature())
	}
	cases := []struct {
		val  any
		want bool
	}{
		{"a@b.com", true},
		{"bad", false},
		{"", true},  // omitempty
		{nil, true}, // omitempty
	}
	for _, c := range cases {
		if got := r.Passes(fakeField{val: reflect.ValueOf(c.val)}); got != c.want {
			t.Errorf("email Passes(%v)=%v want %v", c.val, got, c.want)
		}
	}
}

func TestURLRule(t *testing.T) {
	r := &urlRule{}
	if r.Signature() != "url" {
		t.Fatalf("Signature()=%q", r.Signature())
	}
	cases := []struct {
		val  any
		want bool
	}{
		{"https://example.com", true},
		{"example.com", false},
		{"", true},
	}
	for _, c := range cases {
		if got := r.Passes(fakeField{val: reflect.ValueOf(c.val)}); got != c.want {
			t.Errorf("url Passes(%v)=%v want %v", c.val, got, c.want)
		}
	}
}

func TestURIRule(t *testing.T) {
	r := &uriRule{}
	if r.Signature() != "uri" {
		t.Fatalf("Signature()=%q", r.Signature())
	}
	cases := []struct {
		val  any
		want bool
	}{
		{"mailto:a@b.com", true},
		{"example.com", false},
		{"", true},
	}
	for _, c := range cases {
		if got := r.Passes(fakeField{val: reflect.ValueOf(c.val)}); got != c.want {
			t.Errorf("uri Passes(%v)=%v want %v", c.val, got, c.want)
		}
	}
}

func TestUUIDRule(t *testing.T) {
	r := &uuidRule{}
	if r.Signature() != "uuid" {
		t.Fatalf("Signature()=%q", r.Signature())
	}
	cases := []struct {
		val  any
		want bool
	}{
		{"123e4567-e89b-12d3-a456-426614174000", true},
		{"not-a-uuid", false},
		{"", true},
	}
	for _, c := range cases {
		if got := r.Passes(fakeField{val: reflect.ValueOf(c.val)}); got != c.want {
			t.Errorf("uuid Passes(%v)=%v want %v", c.val, got, c.want)
		}
	}
}

func TestIPRules(t *testing.T) {
	ip := &ipRule{}
	ipv4 := &ipv4Rule{}
	ipv6 := &ipv6Rule{}
	if ip.Signature() != "ip" || ipv4.Signature() != "ipv4" || ipv6.Signature() != "ipv6" {
		t.Fatalf("signatures: %q %q %q", ip.Signature(), ipv4.Signature(), ipv6.Signature())
	}
	if !ip.Passes(fakeField{val: reflect.ValueOf("127.0.0.1")}) || !ip.Passes(fakeField{val: reflect.ValueOf("::1")}) {
		t.Error("ip should accept v4 and v6")
	}
	if ip.Passes(fakeField{val: reflect.ValueOf("nope")}) {
		t.Error("ip should reject garbage")
	}
	if !ip.Passes(fakeField{val: reflect.ValueOf("")}) {
		t.Error("ip omitempty should pass on empty")
	}
	if !ipv4.Passes(fakeField{val: reflect.ValueOf("192.168.0.1")}) || ipv4.Passes(fakeField{val: reflect.ValueOf("::1")}) {
		t.Error("ipv4 accept/reject wrong")
	}
	if !ipv6.Passes(fakeField{val: reflect.ValueOf("::1")}) || ipv6.Passes(fakeField{val: reflect.ValueOf("192.168.0.1")}) {
		t.Error("ipv6 accept/reject wrong")
	}
}

func TestJSONRule(t *testing.T) {
	r := &jsonRule{}
	if r.Signature() != "json" {
		t.Fatalf("Signature()=%q", r.Signature())
	}
	cases := []struct {
		val  any
		want bool
	}{
		{`{"a":1}`, true},
		{`[1,2]`, true},
		{`{bad}`, false},
		{"", true},
	}
	for _, c := range cases {
		if got := r.Passes(fakeField{val: reflect.ValueOf(c.val)}); got != c.want {
			t.Errorf("json Passes(%v)=%v want %v", c.val, got, c.want)
		}
	}
}

func TestBase64Rule(t *testing.T) {
	r := &base64Rule{}
	if r.Signature() != "base64" {
		t.Fatalf("Signature()=%q", r.Signature())
	}
	cases := []struct {
		val  any
		want bool
	}{
		{"aGVsbG8=", true},
		{"not base64!!", false},
		{"", true},
	}
	for _, c := range cases {
		if got := r.Passes(fakeField{val: reflect.ValueOf(c.val)}); got != c.want {
			t.Errorf("base64 Passes(%v)=%v want %v", c.val, got, c.want)
		}
	}
}

func TestMACRule(t *testing.T) {
	r := &macRule{}
	if r.Signature() != "mac" {
		t.Fatalf("Signature()=%q", r.Signature())
	}
	cases := []struct {
		val  any
		want bool
	}{
		{"00:1A:2B:3C:4D:5E", true},
		{"bad", false},
		{"", true},
	}
	for _, c := range cases {
		if got := r.Passes(fakeField{val: reflect.ValueOf(c.val)}); got != c.want {
			t.Errorf("mac Passes(%v)=%v want %v", c.val, got, c.want)
		}
	}
}

func TestHostnameRule(t *testing.T) {
	r := &hostnameRule{}
	if r.Signature() != "hostname" {
		t.Fatalf("Signature()=%q", r.Signature())
	}
	cases := []struct {
		val  any
		want bool
	}{
		{"example.com", true},
		{"-bad.com", false},
		{"", true},
	}
	for _, c := range cases {
		if got := r.Passes(fakeField{val: reflect.ValueOf(c.val)}); got != c.want {
			t.Errorf("hostname Passes(%v)=%v want %v", c.val, got, c.want)
		}
	}
}

func TestDatetimeRule(t *testing.T) {
	r := &datetimeRule{}
	if r.Signature() != "datetime" {
		t.Fatalf("Signature()=%q", r.Signature())
	}
	cases := []formatCase{
		{"2024-01-02", []string{"2006-01-02"}, true},
		{"2024-01-02 15:04:05", []string{"2006-01-02 15:04:05"}, true},
		{"not-a-date", []string{"2006-01-02"}, false},
		{"2024-01-02", []string{"15:04:05"}, false}, // wrong layout
		{"2024-01-02", nil, true},                   // default layouts
		{"garbage", nil, false},
		{"", []string{"2006-01-02"}, true},
	}
	runFormat(t, "datetime", r, cases)
}

func TestDateRule(t *testing.T) {
	r := &dateRule{}
	if r.Signature() != "date" {
		t.Fatalf("Signature()=%q", r.Signature())
	}
	cases := []formatCase{
		{"2024-01-02", nil, true},
		{"2024/01/02", nil, true},
		{"not-a-date", nil, false},
		{"", nil, true},
	}
	runFormat(t, "date", r, cases)
}

func TestRegexRule(t *testing.T) {
	r := &regexRule{}
	if r.Signature() != "regex" {
		t.Fatalf("Signature()=%q", r.Signature())
	}
	cases := []formatCase{
		{"abc", []string{"^[a-z]+$"}, true},
		{"abc123", []string{"^[a-z]+$"}, false},
		{"x", []string{"["}, false}, // invalid pattern
		{"abc", nil, false},         // missing arg
		{"", []string{"^[a-z]+$"}, true},
	}
	runFormat(t, "regex", r, cases)
}

func TestNotRegexRule(t *testing.T) {
	r := &notRegexRule{}
	if r.Signature() != "not_regex" {
		t.Fatalf("Signature()=%q", r.Signature())
	}
	cases := []formatCase{
		{"abc", []string{"[0-9]"}, true},   // no match -> pass
		{"abc1", []string{"[0-9]"}, false}, // match -> fail
		{"x", []string{"["}, false},        // invalid pattern
		{"abc", nil, false},                // missing arg
		{"", []string{"[0-9]"}, true},
	}
	runFormat(t, "not_regex", r, cases)
}

// An invalid regex pattern is a compile-time config error, not a runtime
// "your data is invalid" message.
func TestRegexInvalidPatternFailsFast(t *testing.T) {
	vd := Map(map[string]any{"x": "abc"}, map[string]string{})
	err := vd.AddRules("x", `regex:"["`)
	if err == nil || !strings.Contains(err.Error(), "pattern") {
		t.Errorf("AddRules with an invalid pattern must fail fast mentioning the pattern, got %v", err)
	}

	// via expression compile: reported as a field-level config diagnostic
	bad := Map(map[string]any{"x": "abc"}, map[string]string{"x": `regex:"["`})
	bad.Validate(context.Background())
	if msg := bad.Errors().OneFor("x"); !strings.Contains(msg, "pattern") {
		t.Errorf("an invalid pattern must surface as a config error, got %q", msg)
	}

	// a valid pattern still compiles and validates
	ok := Map(map[string]any{"x": "abc"}, map[string]string{"x": `regex:"^[a-z]+$"`})
	ok.Validate(context.Background())
	if ok.Fails() {
		t.Errorf("valid pattern should pass, got %v", ok.Errors().All())
	}
}

// Arg-less datetime must not render a dangling {0} placeholder.
func TestDatetimeArglessMessage(t *testing.T) {
	vd := Map(map[string]any{"d": "not-a-date"}, map[string]string{"d": "datetime"})
	vd.Validate(context.Background())
	msg := vd.Errors().OneFor("d")
	if msg == "" || strings.Contains(msg, "{0}") {
		t.Errorf("arg-less datetime message must not contain a raw placeholder, got %q", msg)
	}
}
