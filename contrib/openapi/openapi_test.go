package openapi_test

import (
	"errors"
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

	g := openapi.MustNew("test-api", "1.0.0")
	if err := g.Add[paginate](http.MethodGet, "/users", openapi.WithResponse[page](http.StatusOK)); err != nil {
		t.Fatal(err)
	}
	if err := g.Add[userAdd](http.MethodPost, "/users", openapi.WithResponse[user](http.StatusOK)); err != nil {
		t.Fatal(err)
	}
	if err := g.Add[userID](http.MethodGet, "/users/{id}", openapi.WithResponse[user](http.StatusOK)); err != nil {
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

// Embedded structs marshal flat, so the schema must promote their fields too —
// including exported fields of unexported embedded types, like encoding/json.
type listEmbedded struct {
	Page  uint `json:"page" validate:"min:1"`
	Limit uint `json:"limit"`
}
type listRequest struct {
	listEmbedded
	Keyword string `json:"keyword" validate:"required"`
}

func TestEmbeddedStructsFlatten(t *testing.T) {
	g := openapi.MustNew("t", "1")
	if err := g.Add[listRequest](http.MethodPost, "/x", openapi.WithResponse[openapi.NoBody](http.StatusOK)); err != nil {
		t.Fatal(err)
	}
	blob, _ := g.JSON()
	spec := string(blob)
	for _, want := range []string{`"page"`, `"limit"`, `"keyword"`, `"minimum": 1`} {
		if !strings.Contains(spec, want) {
			t.Errorf("embedded fields must be promoted flat (missing %s):\n%s", want, spec)
		}
	}
	if strings.Contains(spec, "listEmbedded") {
		t.Error("the embedded type itself must not appear as a property")
	}
}

// []byte marshals as a base64 string, never an integer array.
func TestByteSliceIsBase64String(t *testing.T) {
	type req struct {
		Payload []byte `json:"payload" validate:"required"`
	}
	g := openapi.MustNew("t", "1")
	_ = g.Add[req](http.MethodPost, "/x", openapi.WithResponse[openapi.NoBody](http.StatusOK))
	blob, _ := g.JSON()
	if !strings.Contains(string(blob), `"contentEncoding": "base64"`) || strings.Contains(string(blob), `"type": "array"`) {
		t.Errorf("[]byte must be a base64 string schema:\n%s", blob)
	}
}

// JSON Schema has no exclusive length bounds: gt/lt on strings and arrays
// shift to the nearest admissible integer instead of silently widening.
func TestExclusiveLengthBounds(t *testing.T) {
	type req struct {
		Name string   `json:"name" validate:"gt:3 && lt:10"`
		Tags []string `json:"tags" validate:"gt:1"`
	}
	g := openapi.MustNew("t", "1")
	_ = g.Add[req](http.MethodPost, "/x", openapi.WithResponse[openapi.NoBody](http.StatusOK))
	blob, _ := g.JSON()
	spec := string(blob)
	for _, want := range []string{`"minLength": 4`, `"maxLength": 9`, `"minItems": 2`} {
		if !strings.Contains(spec, want) {
			t.Errorf("exclusive length bounds must shift by one (missing %s):\n%s", want, spec)
		}
	}
}

// dive rules on a map land on additionalProperties, not items.
func TestMapElementRules(t *testing.T) {
	type req struct {
		Labels map[string]string `json:"labels" validate:"dive && min:2"`
	}
	g := openapi.MustNew("t", "1")
	_ = g.Add[req](http.MethodPost, "/x", openapi.WithResponse[openapi.NoBody](http.StatusOK))
	blob, _ := g.JSON()
	if !strings.Contains(string(blob), `"minLength": 2`) {
		t.Errorf("map element rules must apply to additionalProperties:\n%s", blob)
	}
}

// numeric && gte:18 bounds the VALUE of a numeric string; emitting
// minLength: 18 would demand an 18-character string.
func TestNumericStringHint(t *testing.T) {
	type req struct {
		Age string `json:"age" validate:"numeric && gte:18 && lte:120"`
	}
	g := openapi.MustNew("t", "1")
	_ = g.Add[req](http.MethodPost, "/x", openapi.WithResponse[openapi.NoBody](http.StatusOK))
	blob, _ := g.JSON()
	spec := string(blob)
	if strings.Contains(spec, "minLength") || strings.Contains(spec, "maxLength") {
		t.Errorf("size rules under a numeric hint must not become length bounds:\n%s", spec)
	}
	if !strings.Contains(spec, `"pattern"`) {
		t.Errorf("a numeric string should document its shape as a pattern:\n%s", spec)
	}
}

// unique / port map onto uniqueItems and integer bounds.
func TestUniqueAndPort(t *testing.T) {
	type req struct {
		Ports []int `json:"ports" validate:"unique && dive && port"`
	}
	g := openapi.MustNew("t", "1")
	_ = g.Add[req](http.MethodPost, "/x", openapi.WithResponse[openapi.NoBody](http.StatusOK))
	blob, _ := g.JSON()
	spec := string(blob)
	for _, want := range []string{`"uniqueItems": true`, `"minimum": 1`, `"maximum": 65535`} {
		if !strings.Contains(spec, want) {
			t.Errorf("missing %s:\n%s", want, spec)
		}
	}
}

// A query tag is a query parameter on EVERY method: the binder reads the query
// string on POST too, so documenting it as a body property would misroute
// generated clients.
func TestQueryParamsOnBodyMethods(t *testing.T) {
	type req struct {
		DryRun bool   `query:"dry_run" validate:"boolean"`
		Page   uint   `json:"page" query:"page"` // json+query: parameter wins
		Name   string `json:"name" validate:"required"`
	}
	g := openapi.MustNew("t", "1")
	if err := g.Add[req](http.MethodPost, "/x", openapi.WithResponse[openapi.NoBody](http.StatusOK)); err != nil {
		t.Fatal(err)
	}
	blob, _ := g.JSON()
	spec := string(blob)
	if !strings.Contains(spec, `"in": "query"`) || !strings.Contains(spec, `"dry_run"`) {
		t.Errorf("query-tagged fields must be query parameters on POST:\n%s", spec)
	}
	if strings.Contains(spec, `"DryRun"`) {
		t.Errorf("a query-only field must not leak into the body under its Go name:\n%s", spec)
	}
	if strings.Contains(spec, `"page": {`) && strings.Contains(spec, `"properties"`) &&
		strings.Contains(spec[strings.Index(spec, "properties"):], `"page"`) {
		t.Errorf("a query-tagged field must not also appear as a body property:\n%s", spec)
	}
}

// A malformed validate tag must fail Add loudly — publishing a schema stripped
// of its constraints would be a false contract.
func TestMalformedTagFailsAdd(t *testing.T) {
	type bad struct {
		Name string `json:"name" validate:"required && no_such_rule_here"`
	}
	g := openapi.MustNew("t", "1")
	if err := g.Add[bad](http.MethodPost, "/x", openapi.WithResponse[openapi.NoBody](http.StatusOK)); err == nil {
		t.Fatal("a request type with a malformed validate tag must fail Add")
	}
	// the poisoned operation must not be published
	if len(g.Document().Paths) != 0 {
		t.Error("a failed Add must not register the path")
	}
	// response types are held to the same bar
	g2 := openapi.MustNew("t", "1")
	if err := g2.Add[openapi.NoBody](http.MethodGet, "/y", openapi.WithResponse[bad](http.StatusOK)); err == nil {
		t.Fatal("a response type with a malformed validate tag must fail Add")
	}
}

// required on a struct-valued field must reach the schema's required list —
// runtime rejects a nil profile, so the contract must say so.
func TestRequiredStructField(t *testing.T) {
	type profile struct {
		Bio string `json:"bio" validate:"max:100"`
	}
	type user struct {
		Name    string   `json:"name" validate:"required"`
		Profile *profile `json:"profile" validate:"required"`
	}
	g := openapi.MustNew("t", "1")
	if err := g.Add[user](http.MethodPost, "/x", openapi.WithResponse[openapi.NoBody](http.StatusOK)); err != nil {
		t.Fatal(err)
	}
	blob, _ := g.JSON()
	spec := string(blob)
	if !strings.Contains(spec, `"profile"`) {
		t.Fatalf("profile property missing:\n%s", spec)
	}
	// both name and profile are required body members
	for _, want := range []string{`"name"`, `"profile"`} {
		reqIdx := strings.Index(spec, `"required": [`)
		if reqIdx < 0 || !strings.Contains(spec[reqIdx:], want) {
			t.Errorf("required list must include %s:\n%s", want, spec)
		}
	}
}

// A legal recursive named pointer type must not hang schema generation.
type recPtr *recPtr

func TestRecursivePointerType(t *testing.T) {
	type req struct {
		P recPtr `json:"p"`
	}
	g := openapi.MustNew("t", "1")
	if err := g.Add[req](http.MethodPost, "/x", openapi.WithResponse[openapi.NoBody](http.StatusOK)); err != nil {
		t.Fatal(err)
	}
	blob, err := g.JSON()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(blob), `"p"`) {
		t.Errorf("recursive pointer field should degrade to an open schema:\n%s", blob)
	}
}

// Two distinct types flattening to one component name must not overwrite each
// other's schemas.
type envelopepage struct {
	Other string `json:"other"`
}

func TestComponentNameCollision(t *testing.T) {
	g := openapi.MustNew("t", "1")
	_ = g.Add[openapi.NoBody](http.MethodGet, "/a", openapi.WithResponse[envelope[page]](http.StatusOK)) // flattens to "envelopepage"
	_ = g.Add[openapi.NoBody](http.MethodGet, "/b", openapi.WithResponse[envelopepage](http.StatusOK))   // literal "envelopepage"
	schemas := g.Document().Components.Schemas
	if len(schemas) < 4 { // envelopepage, envelopepage2, page, user
		t.Fatalf("colliding component names must get distinct entries, got %v", keysOf(schemas))
	}
	if _, ok := schemas["envelopepage2"]; !ok {
		t.Errorf("expected a suffixed component for the collision, got %v", keysOf(schemas))
	}
}

func keysOf(m map[string]*openapi.Schema) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

type envelope[T any] struct {
	Msg  string `json:"msg"`
	Data T      `json:"data"`
}

func TestNestedGenericComponentNames(t *testing.T) {
	g := openapi.MustNew("t", "1")
	if err := g.Add[openapi.NoBody](http.MethodGet, "/x", openapi.WithResponse[envelope[page]](http.StatusOK)); err != nil {
		t.Fatal(err)
	}
	blob, _ := g.JSON()
	if !strings.Contains(string(blob), `"#/components/schemas/envelopepage"`) && !strings.Contains(string(blob), `envelopepage`) {
		t.Errorf("nested generic names should flatten cleanly:\n%s", blob)
	}
	if strings.ContainsAny(string(blob), "[]") {
		// brackets may legitimately appear in JSON arrays; check schema keys only
		for name := range g.Document().Components.Schemas {
			if strings.ContainsAny(name, "[]*,") {
				t.Errorf("component name %q contains raw type syntax", name)
			}
		}
	}
}

func TestNew_ValidatesConfiguration(t *testing.T) {
	if _, err := openapi.New("", "1"); !errors.Is(err, openapi.ErrInvalidOption) {
		t.Fatalf("empty title error = %v", err)
	}
	if _, err := openapi.New("title", ""); !errors.Is(err, openapi.ErrInvalidOption) {
		t.Fatalf("empty version error = %v", err)
	}
	if _, err := openapi.New("title", "1", nil); !errors.Is(err, openapi.ErrInvalidOption) {
		t.Fatalf("nil option error = %v", err)
	}
	if _, err := openapi.New("title", "1", openapi.WithValidator(nil)); !errors.Is(err, openapi.ErrInvalidOption) {
		t.Fatalf("nil validator error = %v", err)
	}
}

func TestMustNew_PanicsOnInvalidConfiguration(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("MustNew did not panic")
		}
	}()
	openapi.MustNew("", "1")
}

func TestOperationOptionsAndNoBody(t *testing.T) {
	g := openapi.MustNew("t", "1")
	if err := g.Add[openapi.NoBody](
		http.MethodDelete,
		"/users/{id}",
		openapi.WithSummary("Delete a user"),
		openapi.WithTags("users"),
		openapi.WithResponse[openapi.NoBody](http.StatusNoContent),
	); err != nil {
		t.Fatal(err)
	}
	doc := g.Document()
	operation := doc.Paths["/users/{id}"]["delete"]
	if operation.Summary != "Delete a user" || len(operation.Tags) != 1 {
		t.Fatalf("operation = %+v", operation)
	}
	response := operation.Responses["204"]
	if response == nil || response.Content != nil {
		t.Fatalf("response = %+v", response)
	}
	if err := g.Add[openapi.NoBody](http.MethodDelete, "/users/{id}", openapi.WithResponse[openapi.NoBody](http.StatusOK)); !errors.Is(err, openapi.ErrOperationExists) {
		t.Fatalf("duplicate error = %v", err)
	}
	if err := g.Add[openapi.NoBody]("CONNECT", "/tunnel"); !errors.Is(err, openapi.ErrInvalidOperation) {
		t.Fatalf("unsupported method error = %v", err)
	}
	if err := g.Add[openapi.NoBody](http.MethodGet, "/users?admin=true"); !errors.Is(err, openapi.ErrInvalidOperation) {
		t.Fatalf("path with query error = %v", err)
	}
	if err := g.Add[openapi.NoBody](http.MethodGet, "/nil", nil); !errors.Is(err, openapi.ErrInvalidOption) {
		t.Fatalf("nil operation option error = %v", err)
	}
	if err := g.Add[openapi.NoBody](http.MethodGet, "/status", openapi.WithResponse[openapi.NoBody](99)); !errors.Is(err, openapi.ErrInvalidOperation) {
		t.Fatalf("invalid status error = %v", err)
	}
	if err := g.Add[openapi.NoBody](http.MethodGet, "/missing-response"); !errors.Is(err, openapi.ErrInvalidOperation) {
		t.Fatalf("missing response error = %v", err)
	}
}

func TestMultipleResponses(t *testing.T) {
	type problem struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	g := openapi.MustNew("t", "1")
	err := g.Add[openapi.NoBody](
		http.MethodGet,
		"/users/{id}",
		openapi.WithResponse[user](http.StatusOK),
		openapi.WithResponse[problem](
			http.StatusNotFound,
			openapi.ResponseDescription("User not found"),
		),
		openapi.WithDefaultResponse[problem](),
	)
	if err != nil {
		t.Fatal(err)
	}
	responses := g.Document().Paths["/users/{id}"]["get"].Responses
	if len(responses) != 3 || responses["200"] == nil || responses["404"] == nil || responses["default"] == nil {
		t.Fatalf("responses = %+v", responses)
	}
	if responses["404"].Description != "User not found" {
		t.Fatalf("404 description = %q", responses["404"].Description)
	}
	if err := g.Add[openapi.NoBody](
		http.MethodGet,
		"/duplicate-responses",
		openapi.WithResponse[user](http.StatusOK),
		openapi.WithResponse[problem](http.StatusOK),
	); !errors.Is(err, openapi.ErrInvalidOperation) {
		t.Fatalf("duplicate response error = %v", err)
	}
}

func TestFailedAddDoesNotPolluteComponents(t *testing.T) {
	type good struct {
		ID string `json:"id"`
	}
	type bad struct {
		Value string `json:"value" validate:"unknown_rule"`
	}
	type response struct {
		Good good `json:"good"`
		Bad  bad  `json:"bad"`
	}

	g := openapi.MustNew("t", "1")
	if err := g.Add[openapi.NoBody](
		http.MethodGet,
		"/broken",
		openapi.WithResponse[response](http.StatusOK),
	); err == nil {
		t.Fatal("invalid nested response was accepted")
	}
	doc := g.Document()
	if len(doc.Paths) != 0 || len(doc.Components.Schemas) != 0 {
		t.Fatalf("failed transaction leaked state: paths=%v schemas=%v", doc.Paths, keysOf(doc.Components.Schemas))
	}
	if err := g.Add[openapi.NoBody](
		http.MethodGet,
		"/good",
		openapi.WithResponse[good](http.StatusOK),
	); err != nil {
		t.Fatal(err)
	}
	if _, ok := g.Document().Components.Schemas["good"]; !ok {
		t.Fatalf("failed transaction consumed component name: %v", keysOf(g.Document().Components.Schemas))
	}
}

func TestSchemaOptionAndDocumentAreDefensive(t *testing.T) {
	type identifier string
	schema := &openapi.Schema{Type: "string", Format: "uuid"}
	g := openapi.MustNew("t", "1", openapi.WithSchema[identifier](schema))
	schema.Format = "mutated"
	if err := g.Add[openapi.NoBody](http.MethodGet, "/id", openapi.WithResponse[identifier](http.StatusOK)); err != nil {
		t.Fatal(err)
	}
	doc := g.Document()
	delete(doc.Paths, "/id")
	if _, ok := g.Document().Paths["/id"]; !ok {
		t.Fatal("Document exposed mutable generator state")
	}
	first, err := g.JSON()
	if err != nil {
		t.Fatal(err)
	}
	second, err := g.JSON()
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) || strings.Contains(string(first), "mutated") {
		t.Fatalf("JSON is unstable or schema was aliased:\n%s", first)
	}
	if _, err := openapi.New("t", "1", openapi.WithSchema[int](nil)); !errors.Is(err, openapi.ErrInvalidOption) {
		t.Fatalf("nil schema error = %v", err)
	}
	if _, err := openapi.New("t", "1", openapi.WithSchema[openapi.NoBody](&openapi.Schema{})); !errors.Is(err, openapi.ErrInvalidOption) {
		t.Fatalf("NoBody schema error = %v", err)
	}
}
