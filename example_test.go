package validator_test

import (
	"context"
	"fmt"

	"github.com/libtnb/validator"
)

func Example() {
	type User struct {
		Email string `validate:"required && email"`
		Age   int    `validate:"required && gte:18"`
	}
	vd := validator.Struct(User{Email: "a@b.com", Age: 20})
	vd.Validate(context.Background())
	fmt.Println("valid:", !vd.Fails())
	// Output: valid: true
}

func ExampleVar() {
	vd := validator.Var("nope", "required && email")
	vd.Validate(context.Background())
	fmt.Println(vd.Fails())
	// Output: true
}

func ExampleValidator_Map() {
	v := validator.NewValidator()
	vd := v.Map(
		map[string]any{"name": "alice"},
		// boolean DSL: AND-grouped OR
		map[string]string{"name": "required && (alpha || in:bob,carol)"},
	)
	vd.Validate(context.Background())
	fmt.Println("valid:", !vd.Fails())
	// Output: valid: true
}

func ExampleValidation_SafeBind() {
	type User struct {
		Email string `validate:"required && email"`
		Age   int    `validate:"required && numeric"`
	}
	vd := validator.Map(
		map[string]any{"Email": "a@b.com", "Age": "42"},
		map[string]string{"Email": "required && email", "Age": "required && numeric"},
	)
	vd.Validate(context.Background())

	var u User
	if err := vd.SafeBind(&u); err == nil {
		fmt.Printf("%s / %d\n", u.Email, u.Age)
	}
	// Output: a@b.com / 42
}
