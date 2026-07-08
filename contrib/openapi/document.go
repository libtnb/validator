package openapi

// The OpenAPI 3.1 subset this generator emits. Only what is needed for
// describing JSON APIs is modeled; the zero-dependency spirit of the main
// module applies here too.

type Document struct {
	OpenAPI    string              `json:"openapi"`
	Info       Info                `json:"info"`
	Paths      map[string]PathItem `json:"paths,omitempty"`
	Components *Components         `json:"components,omitempty"`
}

type Info struct {
	Title   string `json:"title"`
	Version string `json:"version"`
}

// PathItem maps a lowercase HTTP method to its operation.
type PathItem map[string]*Operation

type Operation struct {
	Summary     string               `json:"summary,omitempty"`
	Tags        []string             `json:"tags,omitempty"`
	Parameters  []*Parameter         `json:"parameters,omitempty"`
	RequestBody *RequestBody         `json:"requestBody,omitempty"`
	Responses   map[string]*Response `json:"responses"`
}

type Parameter struct {
	Name     string  `json:"name"`
	In       string  `json:"in"` // "path" or "query"
	Required bool    `json:"required,omitempty"`
	Schema   *Schema `json:"schema,omitempty"`
}

type RequestBody struct {
	Required bool                  `json:"required,omitempty"`
	Content  map[string]*MediaType `json:"content"`
}

type MediaType struct {
	Schema *Schema `json:"schema,omitempty"`
}

type Response struct {
	Description string                `json:"description"`
	Content     map[string]*MediaType `json:"content,omitempty"`
}

type Components struct {
	Schemas map[string]*Schema `json:"schemas,omitempty"`
}

// Schema is the JSON Schema subset used by both parameters and bodies.
type Schema struct {
	Ref                  string             `json:"$ref,omitempty"`
	Type                 string             `json:"type,omitempty"`
	Format               string             `json:"format,omitempty"`
	Description          string             `json:"description,omitempty"`
	Properties           map[string]*Schema `json:"properties,omitempty"`
	Items                *Schema            `json:"items,omitempty"`
	AdditionalProperties *Schema            `json:"additionalProperties,omitempty"`
	Required             []string           `json:"required,omitempty"`
	Enum                 []any              `json:"enum,omitempty"`
	Pattern              string             `json:"pattern,omitempty"`
	Minimum              *float64           `json:"minimum,omitempty"`
	Maximum              *float64           `json:"maximum,omitempty"`
	ExclusiveMinimum     *float64           `json:"exclusiveMinimum,omitempty"`
	ExclusiveMaximum     *float64           `json:"exclusiveMaximum,omitempty"`
	MinLength            *uint64            `json:"minLength,omitempty"`
	MaxLength            *uint64            `json:"maxLength,omitempty"`
	MinItems             *uint64            `json:"minItems,omitempty"`
	MaxItems             *uint64            `json:"maxItems,omitempty"`
}
