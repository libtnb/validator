// Command examples is a runnable validator demo; the leading-underscore dir is
// skipped by the go tool, so run it directly: go run ./_examples
package main

import (
	"context"
	"fmt"

	"github.com/libtnb/validator"
	"github.com/libtnb/validator/translations"
)

type SignupForm struct {
	Email    string   `validate:"required && email"`
	Username string   `validate:"required && alphanum && between:3,20"`
	Password string   `validate:"required && min:8"`
	Confirm  string   `validate:"required && same:Password"`
	Role     string   `validate:"required && in:admin,user,guest"`
	Tags     []string `validate:"dive && alpha"`
}

func main() {
	ctx := context.Background()

	good := SignupForm{
		Email: "a@b.com", Username: "alice", Password: "supersecret",
		Confirm: "supersecret", Role: "user", Tags: []string{"go", "dev"},
	}
	demo(ctx, "valid form", good, nil)

	bad := SignupForm{
		Email: "not-an-email", Username: "x", Password: "short",
		Confirm: "different", Role: "root", Tags: []string{"go", "123"},
	}
	demo(ctx, "invalid form", bad, nil)

	zh := validator.NewValidator(validator.WithTranslation(translations.ZhHans()))
	demo(ctx, "invalid form (zh)", bad, zh)
}

func demo(ctx context.Context, title string, form SignupForm, v *validator.Validator) {
	var vd validator.Validation
	if v != nil {
		vd = v.Struct(form)
	} else {
		vd = validator.Struct(form)
	}
	vd.Validate(ctx)

	fmt.Printf("== %s: valid=%v ==\n", title, !vd.Fails())
	for field, rules := range vd.Errors().All() {
		for rule, msg := range rules {
			fmt.Printf("  %s (%s): %s\n", field, rule, msg)
		}
	}
	fmt.Println()
}
