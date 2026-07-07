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

// New format rules: pass/fail/omitempty per rule (empty and nil always pass).
func TestNewFormatRules(t *testing.T) {
	cases := []struct {
		r    Rule
		good []any
		bad  []any
	}{
		{&e164Rule{}, []any{"+14155552671", "+8613800138000"}, []any{"14155552671", "+0123", "+1 415 555", "abc"}},
		{&semverRule{}, []any{"1.2.3", "0.1.0-alpha.1", "2.0.0+build.5"}, []any{"v1.2.3", "1.2", "1.02.3"}},
		{&ulidRule{}, []any{"01ARZ3NDEKTSV4RRFFQ69G5FAV", "01arz3ndektsv4rrffq69g5fav"}, []any{"01ARZ3NDEKTSV4RRFFQ69G5FA", "01ARZ3NDEKTSV4RRFFQ69G5FAI"}},
		{&jwtRule{}, []any{"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.dQw4w9WgXcQ"}, []any{"onlyone", "a.b", "a b.c.d"}},
		{&hexColorRule{}, []any{"#fff", "#FFAA00", "#ffaa00cc", "#abcd"}, []any{"fff", "#ggg", "#12345"}},
		{&fqdnRule{}, []any{"example.com", "sub.example.co.uk", "example.com.", "xn--fiqs8s.cn"}, []any{"localhost", "-bad.com", "bad-.example.com", "bad.-example.com", "example"}},
		{&latitudeRule{}, []any{"45.5", "-90", "+89.9", 45.5, 90}, []any{"90.1", "-91", "abc", 91}},
		{&longitudeRule{}, []any{"180", "-179.99", "0", -73.98}, []any{"180.1", "-181", "abc"}},
		{&timezoneRule{}, []any{"UTC", "Asia/Shanghai"}, []any{"Local", "Not/AZone"}},
		{&portRule{}, []any{"80", 443, "65535", uint16(8080)}, []any{"0", 65536, "-1", "8080.5", "abc"}},
		{&cidrRule{}, []any{"10.0.0.0/8", "2001:db8::/32"}, []any{"10.0.0.0", "10.0.0.0/33", "abc"}},
		{&cidrv4Rule{}, []any{"192.168.1.0/24"}, []any{"2001:db8::/32", "192.168.1.0"}},
		{&cidrv6Rule{}, []any{"2001:db8::/32"}, []any{"192.168.1.0/24"}},
		{&luhnRule{}, []any{"79927398713", "4539578763621486"}, []any{"79927398710", "123a"}},
		{&creditCardRule{}, []any{"4539578763621486", "4539 5787 6362 1486", "4539-5787-6362-1486"}, []any{"4539578763621487", "123", "4539x578763621486"}},
	}
	for _, c := range cases {
		sig := c.r.Signature()
		for _, g := range c.good {
			if !c.r.Passes(fakeField{val: reflect.ValueOf(g)}) {
				t.Errorf("%s(%v) should pass", sig, g)
			}
		}
		for _, b := range c.bad {
			if c.r.Passes(fakeField{val: reflect.ValueOf(b)}) {
				t.Errorf("%s(%v) should fail", sig, b)
			}
		}
		// omitempty: empty string and nil pass
		if !c.r.Passes(fakeField{val: reflect.ValueOf("")}) || !c.r.Passes(fakeField{val: reflect.ValueOf(nil)}) {
			t.Errorf("%s must pass empty values (omitempty)", sig)
		}
	}
}
