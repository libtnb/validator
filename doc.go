// Package validator is a fast Go validator whose headline is a boolean
// rule DSL combining rules with &&, ||, !, () at precedence ! > && > ||
// (e.g. `required && (in:admin,user || regex:"^g.+")`).
//
// Data sources: struct tags, map, JSON, url.Values, Any, or a single Var.
//
// required means present-and-non-nil: a zero value ("", 0, false) passes, a nil
// pointer or absent key fails. Use notblank/filled to reject empty strings, or
// WithStrictRequired to demand a non-zero value.
package validator
