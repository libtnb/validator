// Package gormrules provides exists and not_exists validator rules backed by
// GORM. Expressions use table and column arguments, for example
// "not_exists:users,phone,email".
package gormrules

import (
	"errors"
	"fmt"
	"regexp"

	"gorm.io/gorm"

	"github.com/libtnb/validator"
)

// ErrNilDB reports that New received no database.
var ErrNilDB = errors.New("gormrules: nil database")

// New constructs a Validator with exists and not_exists rules.
func New(db *gorm.DB, options ...validator.Option) (*validator.Validator, error) {
	if db == nil {
		return nil, ErrNilDB
	}
	options = append(options, validator.WithFallibleRules(&Exists{db: db}, &NotExists{db: db}))
	return validator.New(options...)
}

// identifier guards table and column names that get interpolated into SQL.
var identifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Exists validates that a value exists in one of the given table columns.
type Exists struct {
	db *gorm.DB
}

func (r *Exists) Signature() string {
	return "exists"
}

// Message is the fallback template; override it via validator.WithMessages.
func (r *Exists) Message() string {
	return "The {field} does not exist."
}

func (r *Exists) CheckArgs(args []string) error { return checkArgs(args) }

func (r *Exists) Validate(f *validator.Field) (bool, error) {
	count, skip, err := countInTable(r.db, f)
	if err != nil {
		return false, err
	}
	return skip || count > 0, nil
}

// NotExists validates that a value does not exist in any of the given table
// columns.
type NotExists struct {
	db *gorm.DB
}

func (r *NotExists) Signature() string {
	return "not_exists"
}

// Message is the fallback template; override it via validator.WithMessages.
func (r *NotExists) Message() string {
	return "The {field} already exists."
}

func (r *NotExists) CheckArgs(args []string) error { return checkArgs(args) }

func (r *NotExists) Validate(f *validator.Field) (bool, error) {
	count, skip, err := countInTable(r.db, f)
	if err != nil {
		return false, err
	}
	return skip || count == 0, nil
}

// countInTable counts rows of table attrs[0] where any of the attrs[1:]
// columns equals the field value. Absent and zero values are skipped so
// presence stays the business of the required rule.
func countInTable(db *gorm.DB, f *validator.Field) (int64, bool, error) {
	attrs := f.Attrs()
	if len(attrs) < 2 {
		return 0, false, errors.New("rule requires a table and at least one column, e.g. exists:users,id")
	}
	for _, name := range attrs {
		if !identifier.MatchString(name) {
			return 0, false, fmt.Errorf("invalid identifier %q in rule arguments", name)
		}
	}

	rv := f.Reflect()
	if !rv.IsValid() || rv.IsZero() {
		return 0, true, nil
	}

	value := rv.Interface()
	query := db.WithContext(f.Context()).Table(attrs[0]).Where(attrs[1]+" = ?", value)
	for _, column := range attrs[2:] {
		query = query.Or(column+" = ?", value)
	}

	var count int64
	if err := query.Count(&count).Error; err != nil {
		return 0, false, err
	}
	return count, false, nil
}

func checkArgs(attrs []string) error {
	if len(attrs) < 2 {
		return errors.New("requires a table and at least one column")
	}
	for _, name := range attrs {
		if !identifier.MatchString(name) {
			return fmt.Errorf("invalid identifier %q", name)
		}
	}
	return nil
}
