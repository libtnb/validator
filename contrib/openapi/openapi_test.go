package openapi_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/libtnb/validator/contrib/openapi"
)

type userAdd struct {
	Name string   `json:"name" form:"name" validate:"required && notblank && min:3 && max:255"`
	Role string   `json:"role" form:"role" validate:"in:admin,user"`
	Mail string   `json:"mail" form:"mail" validate:"required && email"`
	Tags []string `json:"tags" form:"tags" validate:"dive && min:2"`
}

type userID struct {
	ID uint `uri:"id" validate:"required && number"`
}

type paginate struct {
	Page  uint `json:"page" form:"page" query:"page" validate:"number && min:1 && max:1000000"`
	Limit uint `json:"limit" form:"limit" query:"limit" validate:"number && min:1 && max:1000"`
}

type user struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

type page struct {
	Total int64  `json:"total"`
	Items []user `json:"items"`
}

func build(t *testing.T) string {
	t.Helper()

	g := openapi.New("test-api", "1.0.0")
	if err := g.Add(http.MethodGet, "/users", openapi.Op{Request: paginate{}, Response: page{}}); err != nil {
		t.Fatal(err)
	}
	if err := g.Add(http.MethodPost, "/users", openapi.Op{Request: userAdd{}, Response: user{}}); err != nil {
		t.Fatal(err)
	}
	if err := g.Add(http.MethodGet, "/users/{id}", openapi.Op{Request: userID{}, Response: user{}}); err != nil {
		t.Fatal(err)
	}

	blob, err := g.JSON()
	if err != nil {
		t.Fatal(err)
	}
	return string(blob)
}

func TestConstraintsFromValidateTags(t *testing.T) {
	spec := build(t)

	for _, want := range []string{
		`"minLength": 3`,    // min:3 on a string
		`"maxLength": 255`,  // max:255
		`"format": "email"`, // email rule
		`"enum"`,            // in:admin,user
		`"minimum": 1`,      // min:1 on an integer (query param)
		`"maximum": 1000`,   // max:1000
		`"required"`,        // required rules collected
		`"$ref": "#/components/schemas/user"`,
		`"in": "path"`,  // uri tag -> path parameter
		`"in": "query"`, // query tag on GET -> query parameter
	} {
		if !strings.Contains(spec, want) {
			t.Errorf("spec should contain %s\n%s", want, spec)
		}
	}
}

func TestBodyOmitsPathParams(t *testing.T) {
	spec := build(t)

	// userID.ID is a path parameter; no request body should exist on GET
	if strings.Contains(spec, `"requestBody": {}`) {
		t.Error("GET operations must not carry request bodies")
	}
}

func TestElementRules(t *testing.T) {
	spec := build(t)

	// dive && min:2 on []string -> items.minLength = 2
	if !strings.Contains(spec, `"items"`) {
		t.Errorf("array fields should document items:\n%s", spec)
	}
}

func TestDocsHTML(t *testing.T) {
	html := string(openapi.DocsHTML("test-api", "/openapi.json"))
	if !strings.Contains(html, "/openapi.json") || !strings.Contains(html, "scalar") {
		t.Errorf("docs page should embed the spec url and scalar:\n%s", html)
	}
}
