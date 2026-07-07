package gormrules_test

import (
	"context"
	"testing"

	"github.com/libtnb/sqlite"
	"github.com/libtnb/validator"
	"github.com/libtnb/validator/contrib/gormrules"
	"gorm.io/gorm"
)

type user struct {
	ID   uint `gorm:"primaryKey"`
	Name string
}

func newValidator(t *testing.T) *validator.Validator {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&user{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Create(&user{Name: "alice"}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	v := validator.NewValidator()
	gormrules.Register(v, db)

	return v
}

func TestExists(t *testing.T) {
	v := newValidator(t)

	for value, wantPass := range map[uint]bool{1: true, 999: false} {
		vd := v.Map(map[string]any{"id": value}, map[string]string{"id": "exists:users,id"})
		vd.Validate(context.Background())
		if pass := !vd.Fails(); pass != wantPass {
			t.Errorf("exists id=%d: pass=%v, want %v", value, pass, wantPass)
		}
	}
}

func TestExists_MultipleColumns(t *testing.T) {
	v := newValidator(t)

	vd := v.Map(map[string]any{"q": "alice"}, map[string]string{"q": "exists:users,id,name"})
	vd.Validate(context.Background())
	if vd.Fails() {
		t.Errorf("exists should match any listed column: %v", vd.Errors().All())
	}
}

func TestNotExists(t *testing.T) {
	v := newValidator(t)

	for value, wantPass := range map[string]bool{"bob": true, "alice": false} {
		vd := v.Map(map[string]any{"name": value}, map[string]string{"name": "not_exists:users,name"})
		vd.Validate(context.Background())
		if pass := !vd.Fails(); pass != wantPass {
			t.Errorf("not_exists name=%s: pass=%v, want %v", value, pass, wantPass)
		}
	}
}

func TestSkipsEmptyValues(t *testing.T) {
	v := newValidator(t)

	// zero values are required's business, not the database rules'
	vd := v.Map(map[string]any{"id": 0}, map[string]string{"id": "exists:users,id"})
	vd.Validate(context.Background())
	if vd.Fails() {
		t.Errorf("zero value should be skipped: %v", vd.Errors().All())
	}
}

func TestRejectsBadIdentifier(t *testing.T) {
	v := newValidator(t)

	vd := v.Map(map[string]any{"id": 1}, map[string]string{"id": `exists:users,"id = 1 --"`})
	vd.Validate(context.Background())
	if !vd.Fails() {
		t.Error("an unsafe identifier must fail validation")
	}
}
