# validator

[![Doc](https://pkg.go.dev/badge/github.com/libtnb/validator)](https://pkg.go.dev/github.com/libtnb/validator)
[![Go](https://img.shields.io/github/go-mod/go-version/libtnb/validator)](https://go.dev/)
[![Release](https://img.shields.io/github/release/libtnb/validator.svg)](https://github.com/libtnb/validator/releases)
[![Test](https://github.com/libtnb/validator/actions/workflows/test.yml/badge.svg)](https://github.com/libtnb/validator/actions)
[![Report Card](https://goreportcard.com/badge/github.com/libtnb/validator)](https://goreportcard.com/report/github.com/libtnb/validator)
[![Stars](https://img.shields.io/github/stars/libtnb/validator?style=flat)](https://github.com/libtnb/validator)
[![License](https://img.shields.io/github/license/libtnb/validator)](https://opensource.org/license/MIT)

A modern, zero-dependency, high-performance Go Struct and Field validator with a **boolean rule DSL**.

## Features

- **Zero third-party dependencies** — only the Go standard library.
- **Boolean DSL** — `&&`, `||`, `!`, `()`, precedence `! > && > ||`.
- **Compile-once, run-many** — expressions compile to cached closures.
- **6 entry points** — `Struct`, `Map`, `JSON`, `URLValues`, `Any`, `Var`.
- **And more** — cross-field, diving, bind/SafeBind, `context`, optional concurrency, i18n.

> Requires Go 1.25+.

## Install

```bash
go get github.com/libtnb/validator
```

## Quick start

```go
import (
    "context"
    "fmt"

    "github.com/libtnb/validator"
)

type User struct {
    Email string `validate:"required && email"`
    Pwd   string `validate:"required && min:8"`
    Pwd2  string `validate:"required && same:Pwd"`   // cross-field
    Tags  []string `validate:"dive && alpha"`        // each element
    Role  string `validate:"required && in:admin,user,guest"`
}

user := User{Email: "a@b.com", Pwd: "secret12", Pwd2: "secret12", Role: "user"}

vd := validator.Struct(user)
vd.Validate(context.Background())
if vd.Fails() {
    fmt.Println(vd.Errors().All()) // map[field]map[rule]message
}

var out User
vd.SafeBind(&out) // bind the validated, filtered values into out

// just pass/fail? Valid takes the allocation-free fast path (0 B/op)
if validator.Valid(user) { /* ... */ }
```

Maps / JSON / form values, with explicit rules:

```go
vd := validator.Map(
    map[string]any{"email": "a@b.com", "age": 30},
    map[string]string{
        "email": "required && email",
        "age":   "required && (gte:18 && lte:120)",
    },
)
vd.Validate(context.Background())

validator.JSON(`{"email":"a@b.com"}`, rules)           // decodes then validates
validator.URLValues(r.Form, rules)                     // form data
validator.Var("a@b.com", "required && email")          // a single value
```

## The DSL

| Construct | Meaning |
| --- | --- |
| `a && b` | both must pass |
| `a \|\| b` | at least one must pass |
| `!a` | must NOT pass |
| `(a \|\| b) && c` | grouping; precedence is `! > && > \|\|` |
| `rule:arg1,arg2` | rule with arguments (`in:a,b,c`, `min:3`) |
| `regex:"^(a\|b)+$"` | quoted argument: `\| & ( )` are literal inside quotes |
| `dive` | apply the following rules to each slice/array/map element |

**Argument quoting.** Operators are the double-character forms `&&` / `||`, so a
single `|` inside a regex is never mistaken for OR. For arguments containing
`| & ( )`, wrap them in quotes (`regex:"..."`, only `\"` and `\\` are escapes
inside quotes) or escape with a backslash in bare form (`\| \& \( \)`). Regex
metacharacters like `\d` are preserved.

## Semantics

**Empty values (omitempty).** Non-presence rules pass on empty/zero values, so an
optional field is only checked when present. This is why `!` differs from `not_*`:
`in:a,b` passes on `""` (omitempty), so `!in:a,b` *rejects* `""` while `not_in:a,b`
passes it. Negate a non-presence rule with its `not_*`/`ne` form, not `!` (which is
for presence rules like `!required`).

**`required`.** By default `required` asserts **present and non-nil**: a Go zero
value (`""`, `0`, `false`) counts as provided and **passes**, while a `nil`
pointer or an absent map/JSON key **fails**. To reject empty values, use
`notblank`/`filled` for strings, or `WithStrictRequired()` to make `required`
reject zero values too.

**Sizes: length vs value.** `min`/`max`/`between`/`gt`/`gte`/`lt`/`lte`/`len`/`size`
compare numbers by **value** and strings by **rune count**; add `numeric` to
compare a string by value:

```go
"age": "numeric && gte:18"   // "30" compares as the number 30
"pwd": "required && min:8"   // "999" is 3 characters, fails
```

## Built-in rules

Args are the `rule:arg` values (`—` = none; `…` = repeatable; `?` = optional).

**Presence**

| Rule | Args | Passes when |
| --- | --- | --- |
| `required` | `—` | present and non-nil (non-zero under `WithStrictRequired`) |
| `filled` | `—` | not empty |
| `notblank` | `—` | has a non-whitespace character |
| `sometimes` | `—` | marker: when the field is **absent** (missing key, nil pointer), skip every rule on the field — `"sometimes && required && email"` gives PATCH semantics |
| `required_if` | `field,val…` | required when `field` equals any listed value |
| `required_unless` | `field,val…` | required unless `field` equals any listed value |
| `required_with` | `field…` | required when any listed field is present |
| `required_without` | `field…` | required when any listed field is missing |
| `required_with_all` | `field…` | required when **all** listed fields are present |
| `required_without_all` | `field…` | required when **all** listed fields are missing |
| `excluded_if` | `field,val…` | must be empty when `field` equals any listed value |
| `excluded_unless` | `field,val…` | must be empty unless `field` equals any listed value |
| `excluded_with` | `field…` | must be empty when any listed field is present |
| `excluded_without` | `field…` | must be empty when any listed field is missing |

**String**

| Rule | Args | Passes when |
| --- | --- | --- |
| `alpha` | `—` | letters only |
| `alphanum` | `—` | letters and digits only |
| `ascii` | `—` | ASCII characters only |
| `lowercase` | `—` | all lowercase |
| `uppercase` | `—` | all uppercase |
| `contains` | `sub` | contains the substring |
| `excludes` | `sub` | does not contain the substring |
| `startswith` | `prefix` | has the prefix |
| `endswith` | `suffix` | has the suffix |

**Format**

| Rule | Args | Passes when |
| --- | --- | --- |
| `email` | `—` | a valid email address |
| `url` / `uri` | `—` | a valid URL / URI |
| `uuid` / `ulid` | `—` | a valid UUID / ULID |
| `ip` / `ipv4` / `ipv6` | `—` | a valid IP / IPv4 / IPv6 address |
| `cidr` / `cidrv4` / `cidrv6` | `—` | valid CIDR notation (any / IPv4 / IPv6) |
| `mac` | `—` | a valid MAC address |
| `hostname` / `fqdn` | `—` | a valid RFC 1123 hostname / FQDN (TLD required) |
| `port` | `—` | an integer port in `[1, 65535]` |
| `e164` | `—` | an E.164 phone number (`+14155552671`) |
| `json` | `—` | valid JSON |
| `base64` | `—` | a valid base64 string |
| `jwt` | `—` | a three-segment JWT shape |
| `semver` | `—` | a semantic version (`1.2.3-rc.1`) |
| `hexcolor` | `—` | a hex color (`#fff`, `#ffaa00cc`) |
| `latitude` / `longitude` | `—` | a decimal coordinate in range |
| `timezone` | `—` | an IANA time zone name (`Asia/Shanghai`) |
| `luhn` | `—` | digits passing the Luhn checksum |
| `credit_card` | `—` | a 12-19 digit card number passing Luhn |
| `datetime` | `layout?` | parses with the Go time layout (default layouts if omitted) |
| `date` | `—` | a valid date |
| `regex` | `pattern` | matches the pattern |
| `not_regex` | `pattern` | does not match the pattern |

**Time** — an arg that parses as a date is always a **literal** bound
(`after:2026-01-01`; input keys can never shadow it), otherwise it resolves as
a sibling **field name** (`after:Start`). Values may be `time.Time`, date
strings, or unix timestamps.

| Rule | Args | Passes when |
| --- | --- | --- |
| `after` / `after_or_equal` | `field\|date` | a date after (or equal to) the reference |
| `before` / `before_or_equal` | `field\|date` | a date before (or equal to) the reference |

**File** (`*multipart.FileHeader` fields; `ext` also accepts filename strings)

| Rule | Args | Passes when |
| --- | --- | --- |
| `ext` | `jpg,png,…` | the filename has one of the extensions (case-insensitive) |
| `mimetypes` | `type…` | the **sniffed content** (first 512 bytes, not the client header) matches; `image/*` wildcards work |
| `filemin` / `filemax` | `size` | file size ≥ / ≤ `size` (`512kb`, `10mb`, `1.5gb`; 1024-based) |

Content starting with markup (`<` + letter/`!`/`/`/`?`) is never sniffed as
`text/plain` — `<svg onload=…>` and unlisted tags cannot pass a plain-text
allowlist (stricter than `http.DetectContentType`). Literal `<` in prose
(`a < b`, `<3`) stays plain text.

**Numeric / size** — *size* = value for numbers, rune length for strings (see [above](#semantics)); `n` is a number.

| Rule | Args | Passes when |
| --- | --- | --- |
| `min` / `max` | `n` | size ≥ n / ≤ n |
| `between` | `min,max` | size within `[min, max]` |
| `gt` / `gte` | `n` | size > n / ≥ n |
| `lt` / `lte` | `n` | size < n / ≤ n |
| `len` / `size` | `n` | size == n |
| `digits` | `n` | exactly `n` digits |
| `numeric` | `—` | a number or numeric string |
| `number` | `—` | an integer |
| `boolean` | `—` | convertible to a boolean |

**Comparison**

| Rule | Args | Passes when |
| --- | --- | --- |
| `in` / `in_ci` | `a,b,…` | equals one of the values (`_ci` = case-insensitive) |
| `not_in` | `a,b,…` | equals none of the values |
| `eq` / `ne` | `v` | equals / does not equal `v` |
| `eq_ignore_case` / `ne_ignore_case` | `v` | case-insensitive equals / not equals |
| `unique` | `—` | a slice/array has no duplicate elements (a map no duplicate values) |

**Cross-field**

| Rule | Args | Passes when |
| --- | --- | --- |
| `same` / `eqfield` | `field` | equals another field |
| `different` / `nefield` | `field` | differs from another field |
| `gtfield` / `gtefield` | `field` | > / ≥ another field (numbers, or `time.Time` pairs chronologically) |
| `ltfield` / `ltefield` | `field` | < / ≤ another field (numbers, or `time.Time` pairs chronologically) |
| `confirmed` | `—` | `<field>_confirmation` exists and matches |

**Filters** (transform the value before validation / `SafeBind`)

| Filter | Args | Effect |
| --- | --- | --- |
| `trim` / `ltrim` / `rtrim` | `—` | strip surrounding / leading / trailing whitespace |
| `lower` / `upper` / `title` | `—` | lower-case / upper-case / title-case |
| `int` / `float` / `bool` / `string` | `—` | convert to that type |

The `is` subpackage exposes the format checks as plain functions
(`is.Email`, `is.URL`, ...) for reuse outside the validator.

## Custom rules

`Field.Val()` returns a `reflect.Value` (the value flows without boxing, so the
validation hot path is allocation-free). Read it with the reflect.Value getters,
or `f.Val().Interface()` to get the value as `any` for the `conv` helpers.

```go
v := validator.NewValidator()

// string style — omitempty and string rendering are pre-applied,
// fn holds only the actual check
v.RegisterStringFunc("slug", func(s string, args ...string) bool {
    return !strings.Contains(s, " ")
}, "The {field} must be a slug.")

// function style — read the value via reflect.Value (0-alloc)
v.RegisterFunc("even", func(f validator.Field) bool {
    rv := f.Val()
    return rv.CanInt() && rv.Int()%2 == 0
}, "The {field} must be even.")

// interface style: implement Signature() / Passes(Field) bool / Message()
v.RegisterRule(&MyRule{})
```

Rules that need a `context.Context` or return an error (e.g. a DB uniqueness
check) implement `validator.ErrorRule` instead and read `f.Context()`.

## Structured errors & tag linting

```go
vd.Errors().Items()             // []FieldError{Field, Rule, Message, Params} — build API error payloads
es, ok := validator.AsErrors(err) // recover the collection from a (wrapped) Validation.Err()

// catch tag typos in a test instead of at request time:
// unknown rules, DSL syntax errors and bad static args, all fields at once
func TestRequestTags(t *testing.T) {
    if err := validator.CheckRules(CreateUserRequest{}); err != nil {
        t.Fatal(err)
    }
}
```

## Messages, attributes & i18n

```go
v := validator.NewValidator(
    validator.WithAttributes(map[string]string{"email": "Email address"}),
    validator.WithMessages(map[string]string{
        "email.required": "Please provide your email.", // field-level
        "required":       "This field is required.",     // rule-level
    }),
    validator.WithTranslation(translations.ZhHans()),    // i18n fallback
)
```

Locale packs (`ZhHans`, `ZhHant`, `Ja`, `Ko`, `Es`) live in the
`github.com/libtnb/validator/translations` subpackage.

Templates use `{field}` (replaced by the attribute alias or field name) and
`{0}`, `{1}`, ... (the rule arguments). `Validation.AddMessages` overrides
templates for a single run:

```go
vd := v.Struct(req)
vd.AddMessages(map[string]string{"email.required": "We need your email."})
```

Priority: `AddMessages > WithMessages > i18n > built-in English`; within a map,
`field.rule` beats `rule`.

## License

See [LICENSE](LICENSE).
