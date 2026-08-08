# validator

[![Go Reference](https://pkg.go.dev/badge/github.com/libtnb/validator.svg)](https://pkg.go.dev/github.com/libtnb/validator)
[![Test](https://github.com/libtnb/validator/actions/workflows/test.yml/badge.svg)](https://github.com/libtnb/validator/actions)

Struct and request-data validation with a boolean rule DSL, typed field access,
atomic binding, filters, and localized messages. The root module uses only the
standard library and requires Go 1.27.

```sh
go get github.com/libtnb/validator
```

## Usage

```go
type User struct {
	Email string   `validate:"required && email"`
	Age   int      `validate:"required && gte:18"`
	Tags  []string `validate:"dive && alpha"`
}

validation, err := validator.Struct(User{Email: "a@b.com", Age: 20})
if err != nil {
	log.Fatal(err) // invalid input or rule configuration
}

var user User
if err := validation.ValidateAs(ctx, &user); err != nil {
	if fields, ok := validator.AsErrors(err); ok {
		fmt.Println(fields.All())
	} else {
		log.Printf("validation could not complete: %v", err)
	}
}
```

`ValidateAs` validates and atomically binds filtered values. `Bind` performs an
atomic raw bind without validation. The destination is unchanged on error.

## Inputs

| Constructor | Input |
| --- | --- |
| `Struct` | Struct value or pointer; rules come from tags |
| `Map` | Any Go map plus field expressions |
| `JSON` | JSON object plus field expressions |
| `Values` | `url.Values` plus field expressions |
| `Value` | One value and one expression |

Constructors return input and rule-configuration errors immediately. Their
`Must*` forms are intended for static, known-valid configuration. JSON rejects
duplicate names, invalid UTF-8, trailing data, and non-object roots while
preserving integer precision.

## Expressions

```text
required && (email || regex:"^[a-z]+@example\\.com$")
```

The DSL supports `!`, `&&`, `||`, parentheses, arguments, and `dive` for
collection elements. Precedence is `!`, `&&`, then `||`.

- `required` checks presence/non-nil; `WithStrictRequired` also rejects zero values.
- Most other rules ignore empty values. Use `filled` or `notblank` when emptiness must fail.
- Size rules compare numbers by value and strings by rune count. A `numeric`
  assertion makes numeric strings use numeric bounds.
- `sometimes` skips an absent field, which is useful for PATCH inputs.

Built-ins cover presence, strings, formats, numbers, collections, cross-field
comparisons, time, files, and scalar filters. `Rules` and `Filters` return
defensive catalog copies.

## Custom rules

Validators are immutable after construction and safe to share:

```go
v, err := validator.New(validator.WithRuleFunc(
	"even",
	func(field *validator.Field) bool {
		value, ok := field.Value[int]()
		return ok && value%2 == 0
	},
	"The {field} must be even.",
))
```

Implement `Rule` for pure boolean checks. Implement `FallibleRule` for checks
that call a database or service: `false, nil` means invalid data, while a
non-nil error means the check could not complete. Operational errors are
returned as `*RuleError` and never inserted into the field `Errors` collection.
A pooled `Field` must not be retained after the call.

`Validation` may be configured with rules, filters, and message overrides until
validation starts. Later mutations return `ErrValidated`. Use `Check[T]` at
startup to compile and verify all tags on a struct type.

## Optional modules

OpenAPI 3.1 generation:

```go
g := openapi.MustNew("users", "1.0.0")
err := g.Add[CreateUser](
	http.MethodPost,
	"/users",
	openapi.WithResponse[User](http.StatusCreated),
	openapi.WithResponse[Problem](http.StatusBadRequest),
)
```

Operation generation is transactional, and multiple responses may be declared
in one call. `contrib/gormrules.New` constructs a Validator with database-backed
`exists` and `not_exists` rules. Both are separate Go modules.

Package-level helpers use a shared immutable built-in Validator. Construct your
own Validator when rules, translations, or other options differ.

API details and examples are on
[pkg.go.dev](https://pkg.go.dev/github.com/libtnb/validator).
