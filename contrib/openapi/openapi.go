// Package openapi generates OpenAPI 3.1 documents from Go types and validator tags.
package openapi

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"reflect"
	"slices"
	"strings"

	"github.com/libtnb/validator"
)

// Errors returned for invalid generator configuration and operations.
var (
	ErrInvalidOption    = errors.New("openapi: invalid option")
	ErrInvalidOperation = errors.New("openapi: invalid operation")
	ErrOperationExists  = errors.New("openapi: operation already exists")
)

// NoBody marks a request or response without JSON content.
type NoBody struct{}

var noBodyType = reflect.TypeFor[NoBody]()

// Generator accumulates operations. It is not safe for concurrent use.
type Generator struct {
	doc      *Document
	v        *validator.Validator
	named    map[reflect.Type]string
	owner    map[string]reflect.Type
	override map[reflect.Type]*Schema
	rules    map[reflect.Type]map[string]validator.FieldRules
}

// Option configures a Generator.
type Option func(*Generator) error

// WithValidator selects the Validator used to inspect validate tags.
func WithValidator(v *validator.Validator) Option {
	return func(g *Generator) error {
		if v == nil {
			return fmt.Errorf("%w: validator is nil", ErrInvalidOption)
		}
		g.v = v
		return nil
	}
}

// New constructs a Generator.
func New(title, version string, opts ...Option) (*Generator, error) {
	if strings.TrimSpace(title) == "" {
		return nil, fmt.Errorf("%w: title is empty", ErrInvalidOption)
	}
	if strings.TrimSpace(version) == "" {
		return nil, fmt.Errorf("%w: version is empty", ErrInvalidOption)
	}
	g := &Generator{
		doc: &Document{
			OpenAPI: "3.1.0",
			Info:    Info{Title: title, Version: version},
			Paths:   map[string]PathItem{},
			Components: &Components{
				Schemas: map[string]*Schema{},
			},
		},
		v:        validator.Default(),
		named:    map[reflect.Type]string{},
		owner:    map[string]reflect.Type{},
		override: map[reflect.Type]*Schema{},
		rules:    map[reflect.Type]map[string]validator.FieldRules{},
	}
	for i, opt := range opts {
		if opt == nil {
			return nil, fmt.Errorf("%w: option %d is nil", ErrInvalidOption, i)
		}
		if err := opt(g); err != nil {
			return nil, err
		}
	}
	return g, nil
}

// MustNew is New with panic-on-error semantics.
func MustNew(title, version string, opts ...Option) *Generator {
	g, err := New(title, version, opts...)
	if err != nil {
		panic(err)
	}
	return g
}

// WithSchema replaces the inferred schema for T. The input is copied.
func WithSchema[T any](schema *Schema) Option {
	return func(g *Generator) error {
		if schema == nil {
			return fmt.Errorf("%w: schema is nil", ErrInvalidOption)
		}
		t := derefType(reflect.TypeFor[T]())
		if t == noBodyType {
			return fmt.Errorf("%w: NoBody cannot have a schema", ErrInvalidOption)
		}
		g.override[t] = cloneSchema(schema)
		return nil
	}
}

type operationConfig struct {
	summary   string
	tags      []string
	responses []responseConfig
}

type responseConfig struct {
	code        string
	description string
	body        reflect.Type
}

// OperationOption configures one operation.
type OperationOption func(*operationConfig) error

// WithSummary sets the operation summary.
func WithSummary(summary string) OperationOption {
	return func(config *operationConfig) error {
		config.summary = strings.TrimSpace(summary)
		return nil
	}
}

// WithTags appends non-empty operation tags.
func WithTags(tags ...string) OperationOption {
	return func(config *operationConfig) error {
		for i, tag := range tags {
			tag = strings.TrimSpace(tag)
			if tag == "" {
				return fmt.Errorf("%w: tag %d is empty", ErrInvalidOption, i)
			}
			config.tags = append(config.tags, tag)
		}
		return nil
	}
}

type responseOptions struct {
	description string
}

// ResponseOption configures one response.
type ResponseOption func(*responseOptions) error

// ResponseDescription overrides the default HTTP status description.
func ResponseDescription(description string) ResponseOption {
	return func(config *responseOptions) error {
		description = strings.TrimSpace(description)
		if description == "" {
			return fmt.Errorf("%w: response description is empty", ErrInvalidOption)
		}
		config.description = description
		return nil
	}
}

// WithResponse adds a status response whose JSON body is Body. Use NoBody for
// responses without content.
func WithResponse[Body any](status int, opts ...ResponseOption) OperationOption {
	return responseOption[Body](
		fmt.Sprintf("%d", status),
		status,
		opts,
	)
}

// WithDefaultResponse adds the OpenAPI "default" response.
func WithDefaultResponse[Body any](opts ...ResponseOption) OperationOption {
	return responseOption[Body]("default", 0, opts)
}

// Add registers an operation. At least one WithResponse or
// WithDefaultResponse option is required. Generation is transactional: an
// error leaves paths, components, and schema-name allocation unchanged.
func (g *Generator) Add[Request any](method, path string, opts ...OperationOption) error {
	method = strings.ToLower(strings.TrimSpace(method))
	path = strings.TrimSpace(path)
	validPath := strings.HasPrefix(path, "/") && !strings.ContainsAny(path, "?#")
	if !validMethod(method) || !validPath {
		return fmt.Errorf("%w: method and absolute path are required", ErrInvalidOperation)
	}
	if item, ok := g.doc.Paths[path]; ok && item[method] != nil {
		return fmt.Errorf("%w: %s %s", ErrOperationExists, method, path)
	}
	config := operationConfig{}
	for i, opt := range opts {
		if opt == nil {
			return fmt.Errorf("%w: operation option %d is nil", ErrInvalidOption, i)
		}
		if err := opt(&config); err != nil {
			return err
		}
	}
	if len(config.responses) == 0 {
		return fmt.Errorf("%w: at least one response is required", ErrInvalidOperation)
	}
	seenResponses := make(map[string]struct{}, len(config.responses))
	for _, response := range config.responses {
		if _, duplicate := seenResponses[response.code]; duplicate {
			return fmt.Errorf("%w: duplicate response %q", ErrInvalidOperation, response.code)
		}
		seenResponses[response.code] = struct{}{}
	}

	tx := g.transaction()
	operation := &Operation{
		Summary:   config.summary,
		Tags:      slices.Clone(config.tags),
		Responses: make(map[string]*Response, len(config.responses)),
	}
	requestType := derefType(reflect.TypeFor[Request]())
	if requestType != noBodyType {
		if err := tx.request(method, requestType, operation); err != nil {
			return fmt.Errorf("%s %s: %w", method, path, err)
		}
	}
	for _, configured := range config.responses {
		response := &Response{Description: configured.description}
		if configured.body != noBodyType {
			schema, err := tx.schemaOf(configured.body)
			if err != nil {
				return fmt.Errorf("%s %s response %s: %w", method, path, configured.code, err)
			}
			response.Content = map[string]*MediaType{
				"application/json": {Schema: schema},
			}
		}
		operation.Responses[configured.code] = response
	}
	item := maps.Clone(tx.doc.Paths[path])
	if item == nil {
		item = PathItem{}
	}
	tx.doc.Paths[path] = item
	item[method] = operation
	g.commit(tx)
	return nil
}

// Document returns a deep copy.
func (g *Generator) Document() Document { return cloneDocument(*g.doc) }

// JSON returns deterministic, indented JSON.
func (g *Generator) JSON() ([]byte, error) {
	return json.Marshal(
		g.doc,
		json.Deterministic(true),
		jsontext.WithIndent("  "),
	)
}

func (g *Generator) transaction() *Generator {
	doc := *g.doc
	doc.Paths = maps.Clone(g.doc.Paths)
	if g.doc.Components != nil {
		components := *g.doc.Components
		components.Schemas = maps.Clone(g.doc.Components.Schemas)
		doc.Components = &components
	}
	return &Generator{
		doc: &doc, v: g.v,
		named: maps.Clone(g.named), owner: maps.Clone(g.owner),
		override: maps.Clone(g.override), rules: maps.Clone(g.rules),
	}
}

func (g *Generator) commit(tx *Generator) {
	g.doc = tx.doc
	g.named = tx.named
	g.owner = tx.owner
	g.override = tx.override
	g.rules = tx.rules
}

func (g *Generator) request(method string, t reflect.Type, operation *Operation) error {
	if t.Kind() != reflect.Struct {
		return fmt.Errorf("request type must be a struct, got %s", t)
	}
	rules, err := g.rulesFor(t)
	if err != nil {
		return err
	}
	hasBody := method == strings.ToLower(http.MethodPost) ||
		method == strings.ToLower(http.MethodPut) ||
		method == strings.ToLower(http.MethodPatch)
	body := &Schema{Type: "object", Properties: map[string]*Schema{}}
	for _, field := range jsonFields(t, nil, map[reflect.Type]bool{}) {
		fieldRules := rules[indexKey(field.index)]
		if name, _, _ := strings.Cut(field.Tag.Get("uri"), ","); name != "" && name != "-" {
			schema, err := g.schemaOf(field.Type)
			if err != nil {
				return err
			}
			applyRules(schema, fieldRules.Rules)
			operation.Parameters = append(operation.Parameters, &Parameter{
				Name: name, In: "path", Required: true, Schema: schema,
			})
			continue
		}
		if name, _, _ := strings.Cut(field.Tag.Get("query"), ","); name != "" && name != "-" {
			schema, err := g.schemaOf(field.Type)
			if err != nil {
				return err
			}
			required := applyRules(schema, fieldRules.Rules)
			operation.Parameters = append(operation.Parameters, &Parameter{
				Name: name, In: "query", Required: required, Schema: schema,
			})
			continue
		}
		if !hasBody || field.name == "" {
			continue
		}
		if _, exists := body.Properties[field.name]; exists {
			continue
		}
		property, err := g.schemaOf(field.Type)
		if err != nil {
			return err
		}
		if applyRules(property, fieldRules.Rules) {
			body.Required = append(body.Required, field.name)
		}
		applyElementRules(property, fieldRules.Element)
		body.Properties[field.name] = property
	}
	if hasBody && len(body.Properties) > 0 {
		operation.RequestBody = &RequestBody{
			Required: len(body.Required) > 0,
			Content:  map[string]*MediaType{"application/json": {Schema: body}},
		}
	}
	return nil
}

func (g *Generator) rulesFor(t reflect.Type) (map[string]validator.FieldRules, error) {
	if cached, ok := g.rules[t]; ok {
		return cached, nil
	}
	fields, err := g.v.DescribeType(t)
	if err == nil {
		err = g.v.CheckType(t)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: invalid validate tags: %w", t, err)
	}
	out := make(map[string]validator.FieldRules, len(fields))
	for _, field := range fields {
		out[indexKey(field.Index)] = field
	}
	g.rules[t] = out
	return out, nil
}

func responseOption[Body any](code string, status int, opts []ResponseOption) OperationOption {
	return func(operation *operationConfig) error {
		if code != "default" && (status < 100 || status > 599) {
			return fmt.Errorf("%w: invalid response status %d", ErrInvalidOperation, status)
		}
		config := responseOptions{}
		for i, opt := range opts {
			if opt == nil {
				return fmt.Errorf("%w: response option %d is nil", ErrInvalidOption, i)
			}
			if err := opt(&config); err != nil {
				return err
			}
		}
		if config.description == "" {
			config.description = http.StatusText(status)
			if config.description == "" {
				config.description = "Response"
			}
		}
		operation.responses = append(operation.responses, responseConfig{
			code:        code,
			description: config.description,
			body:        derefType(reflect.TypeFor[Body]()),
		})
		return nil
	}
}

func validMethod(method string) bool {
	switch method {
	case "get", "put", "post", "delete", "options", "head", "patch", "trace":
		return true
	default:
		return false
	}
}

func indexKey(index []int) string { return fmt.Sprint(index) }
