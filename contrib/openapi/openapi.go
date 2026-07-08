// Package openapi generates OpenAPI 3.1 documents from plain Go types: the
// same struct tags that drive github.com/libtnb/validator (validate, json,
// uri, query) become schemas, parameters and constraints — the validation
// rules are the documentation, so the two can never drift apart.
//
//	g := openapi.New("my-api", "1.0.0")
//	g.Add(http.MethodPost, "/users", openapi.Op{
//	    Summary:  "Create a user",
//	    Request:  request.UserAdd{},   // validate:"required && min:3" -> minLength: 3
//	    Response: biz.User{},
//	})
//	blob, _ := g.JSON()
package openapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/libtnb/validator"
)

// Generator accumulates operations into an OpenAPI 3.1 document.
type Generator struct {
	doc      *Document
	v        *validator.Validator
	named    map[reflect.Type]string
	override map[reflect.Type]*Schema
	rules    map[reflect.Type]map[string]validator.FieldRules
}

type Option func(*Generator)

// WithValidator uses v to introspect validation rules. Default: the
// package-level default validator.
func WithValidator(v *validator.Validator) Option {
	return func(g *Generator) { g.v = v }
}

// WithType overrides the schema of a type the reflector cannot guess,
// e.g. WithType(carbon.DateTime{}, &Schema{Type: "string", Format: "date-time"}).
func WithType(sample any, s *Schema) Option {
	return func(g *Generator) { g.override[reflect.TypeOf(sample)] = s }
}

func New(title, version string, opts ...Option) *Generator {
	g := &Generator{
		doc: &Document{
			OpenAPI:    "3.1.0",
			Info:       Info{Title: title, Version: version},
			Paths:      map[string]PathItem{},
			Components: &Components{Schemas: map[string]*Schema{}},
		},
		v:        validator.Default(),
		named:    map[reflect.Type]string{},
		override: map[reflect.Type]*Schema{},
		rules:    map[reflect.Type]map[string]validator.FieldRules{},
	}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// Op describes one operation. Request and Response are sample values (or
// zero structs); their types drive everything else.
type Op struct {
	Summary  string
	Tags     []string
	Request  any // uri tags -> path params, query tags -> query params, json tags -> body
	Response any // the 200-response body; nil means no content
	Status   int // response status, default 200
}

// Add registers an operation under an OAS-style path ("/users/{id}").
func (g *Generator) Add(method, path string, op Op) error {
	operation := &Operation{
		Summary:   op.Summary,
		Tags:      op.Tags,
		Responses: map[string]*Response{},
	}

	if op.Request != nil {
		if err := g.request(method, op.Request, operation); err != nil {
			return fmt.Errorf("%s %s: %w", method, path, err)
		}
	}

	status := op.Status
	if status == 0 {
		status = http.StatusOK
	}
	resp := &Response{Description: http.StatusText(status)}
	if op.Response != nil {
		resp.Content = map[string]*MediaType{
			"application/json": {Schema: g.schemaOf(reflect.TypeOf(op.Response))},
		}
	}
	operation.Responses[fmt.Sprintf("%d", status)] = resp

	item, ok := g.doc.Paths[path]
	if !ok {
		item = PathItem{}
		g.doc.Paths[path] = item
	}
	item[strings.ToLower(method)] = operation

	return nil
}

// request splits a bound struct the same way service.Bind does: uri tags are
// path parameters, query tags are query parameters, json tags form the body
// on methods that carry one.
func (g *Generator) request(method string, sample any, op *Operation) error {
	t := reflect.TypeOf(sample)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return fmt.Errorf("request sample must be a struct, got %s", t)
	}

	rules := g.rulesFor(t)
	hasBody := method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch

	body := &Schema{Type: "object", Properties: map[string]*Schema{}}
	for i := range t.NumField() {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		fr := rules[indexKey(field.Index)]

		if name, _, _ := strings.Cut(field.Tag.Get("uri"), ","); name != "" && name != "-" {
			schema := g.schemaOf(field.Type)
			applyRules(schema, fr.Rules, field.Type)
			op.Parameters = append(op.Parameters, &Parameter{
				Name: name, In: "path", Required: true, Schema: schema,
			})
			continue
		}

		if name, _, _ := strings.Cut(field.Tag.Get("query"), ","); name != "" && name != "-" && !hasBody {
			schema := g.schemaOf(field.Type)
			required := applyRules(schema, fr.Rules, field.Type)
			op.Parameters = append(op.Parameters, &Parameter{
				Name: name, In: "query", Required: required, Schema: schema,
			})
			continue
		}

		if !hasBody {
			continue
		}
		if name := jsonName(field); name != "" {
			prop := g.schemaOf(field.Type)
			if applyRules(prop, fr.Rules, field.Type) {
				body.Required = append(body.Required, name)
			}
			if len(fr.Element) > 0 && prop.Items != nil {
				applyRules(prop.Items, fr.Element, field.Type.Elem())
			}
			body.Properties[name] = prop
		}
	}

	if hasBody && len(body.Properties) > 0 {
		op.RequestBody = &RequestBody{
			Required: len(body.Required) > 0,
			Content:  map[string]*MediaType{"application/json": {Schema: body}},
		}
	}

	return nil
}

// rulesFor introspects and caches the validate rules of a struct type,
// keyed by field index path.
func (g *Generator) rulesFor(t reflect.Type) map[string]validator.FieldRules {
	if cached, ok := g.rules[t]; ok {
		return cached
	}

	out := map[string]validator.FieldRules{}
	fields, err := g.v.DescribeRules(reflect.New(t).Elem().Interface())
	if err == nil {
		for _, fr := range fields {
			out[indexKey(fr.Index)] = fr
		}
	}
	g.rules[t] = out

	return out
}

func indexKey(index []int) string {
	return fmt.Sprint(index)
}

// Document returns the assembled document.
func (g *Generator) Document() *Document {
	return g.doc
}

// JSON marshals the document, indented for humans and diffs.
func (g *Generator) JSON() ([]byte, error) {
	return json.MarshalIndent(g.doc, "", "  ")
}
