// Package gormrules provides database-backed validation rules on top of
// gorm: exists and not_exists check a value against table columns.
//
// Usage in a rule expression:
//
//	exists:table,column[,column...]        the value exists in any column
//	not_exists:table,column[,column...]    the value exists in no column
//
// Example: `validate:"required && not_exists:users,phone,email"`.
//
// Register both on a validator with Register(v, db). Messages default to
// English; override them per validator with validator.WithMessages, e.g.
// {"exists": "{field} 不存在", "not_exists": "{field} 已存在"}.
package gormrules

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/libtnb/validator"
	"gorm.io/gorm"
)

// Register adds the database-backed rules (exists, not_exists) to v.
func Register(v *validator.Validator, db *gorm.DB) {
	v.RegisterErrorRule(NewExists(db))
	v.RegisterErrorRule(NewNotExists(db))
}

// identifier guards table and column names that get interpolated into SQL.
var identifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// countInTable counts rows of table attrs[0] where any of the attrs[1:]
// columns equals the field value. Absent and zero values are skipped so
// presence stays the business of the required rule.
func countInTable(db *gorm.DB, f validator.Field) (count int64, skip bool, err error) {
	attrs := f.Attrs()
	if len(attrs) < 2 {
		return 0, false, errors.New("rule requires a table and at least one column, e.g. exists:users,id")
	}
	for _, name := range attrs {
		if !identifier.MatchString(name) {
			return 0, false, fmt.Errorf("invalid identifier %q in rule arguments", name)
		}
	}

	rv := f.Val()
	if !rv.IsValid() || rv.IsZero() {
		return 0, true, nil
	}

	val := rv.Interface()
	query := db.WithContext(f.Context()).Table(attrs[0]).Where(attrs[1]+" = ?", val)
	for _, column := range attrs[2:] {
		query = query.Or(column+" = ?", val)
	}

	if err = query.Count(&count).Error; err != nil {
		return 0, false, err
	}

	return count, false, nil
}

// Exists validates that a value exists in one of the given table columns.
type Exists struct {
	db *gorm.DB
}

func NewExists(db *gorm.DB) *Exists {
	return &Exists{db: db}
}

func (r *Exists) Signature() string {
	return "exists"
}

// Message is the fallback template; override it via validator.WithMessages.
func (r *Exists) Message() string {
	return "The {field} does not exist."
}

func (r *Exists) PassesE(f validator.Field) error {
	count, skip, err := countInTable(r.db, f)
	if err != nil {
		return err
	}
	if !skip && count == 0 {
		return errors.New(r.Message())
	}

	return nil
}

// NotExists validates that a value does not exist in any of the given table
// columns.
type NotExists struct {
	db *gorm.DB
}

func NewNotExists(db *gorm.DB) *NotExists {
	return &NotExists{db: db}
}

func (r *NotExists) Signature() string {
	return "not_exists"
}

// Message is the fallback template; override it via validator.WithMessages.
func (r *NotExists) Message() string {
	return "The {field} already exists."
}

func (r *NotExists) PassesE(f validator.Field) error {
	count, skip, err := countInTable(r.db, f)
	if err != nil {
		return err
	}
	if !skip && count != 0 {
		return errors.New(r.Message())
	}

	return nil
}
