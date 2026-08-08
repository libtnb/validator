package openapi

import "slices"

// Document is the generated OpenAPI 3.1 document.
type Document struct {
	OpenAPI    string              `json:"openapi"`
	Info       Info                `json:"info"`
	Paths      map[string]PathItem `json:"paths,omitempty"`
	Components *Components         `json:"components,omitempty"`
}

// Info identifies the API.
type Info struct {
	Title   string `json:"title"`
	Version string `json:"version"`
}

// PathItem maps a lowercase HTTP method to its operation.
type PathItem map[string]*Operation

// Operation describes one HTTP operation.
type Operation struct {
	Summary     string               `json:"summary,omitempty,omitzero"`
	Tags        []string             `json:"tags,omitempty"`
	Parameters  []*Parameter         `json:"parameters,omitempty"`
	RequestBody *RequestBody         `json:"requestBody,omitempty"`
	Responses   map[string]*Response `json:"responses"`
}

// Parameter describes a path or query parameter.
type Parameter struct {
	Name     string  `json:"name"`
	In       string  `json:"in"` // "path" or "query"
	Required bool    `json:"required,omitempty,omitzero"`
	Schema   *Schema `json:"schema,omitempty"`
}

// RequestBody describes a JSON request body.
type RequestBody struct {
	Required bool                  `json:"required,omitempty,omitzero"`
	Content  map[string]*MediaType `json:"content"`
}

// MediaType associates content with a schema.
type MediaType struct {
	Schema *Schema `json:"schema,omitempty"`
}

// Response describes one HTTP response.
type Response struct {
	Description string                `json:"description"`
	Content     map[string]*MediaType `json:"content,omitempty"`
}

// Components contains reusable schemas.
type Components struct {
	Schemas map[string]*Schema `json:"schemas,omitempty"`
}

// Schema is the JSON Schema subset used by both parameters and bodies.
type Schema struct {
	Ref                  string             `json:"$ref,omitempty,omitzero"`
	Type                 string             `json:"type,omitempty,omitzero"`
	Format               string             `json:"format,omitempty,omitzero"`
	Description          string             `json:"description,omitempty,omitzero"`
	Properties           map[string]*Schema `json:"properties,omitempty"`
	Items                *Schema            `json:"items,omitempty"`
	AdditionalProperties *Schema            `json:"additionalProperties,omitempty"`
	Required             []string           `json:"required,omitempty"`
	Enum                 []any              `json:"enum,omitempty"`
	Pattern              string             `json:"pattern,omitempty,omitzero"`
	Minimum              *float64           `json:"minimum,omitempty"`
	Maximum              *float64           `json:"maximum,omitempty"`
	ExclusiveMinimum     *float64           `json:"exclusiveMinimum,omitempty"`
	ExclusiveMaximum     *float64           `json:"exclusiveMaximum,omitempty"`
	MinLength            *uint64            `json:"minLength,omitempty"`
	MaxLength            *uint64            `json:"maxLength,omitempty"`
	MinItems             *uint64            `json:"minItems,omitempty"`
	MaxItems             *uint64            `json:"maxItems,omitempty"`
	UniqueItems          bool               `json:"uniqueItems,omitempty,omitzero"`
	// ContentEncoding marks base64 payloads ([]byte fields), per JSON Schema 2020-12.
	ContentEncoding string `json:"contentEncoding,omitempty,omitzero"`
}

func cloneDocument(document Document) Document {
	clone := Document{OpenAPI: document.OpenAPI, Info: document.Info}
	seen := make(map[*Schema]*Schema)
	if document.Components != nil {
		clone.Components = &Components{Schemas: make(map[string]*Schema, len(document.Components.Schemas))}
		for name, schema := range document.Components.Schemas {
			clone.Components.Schemas[name] = cloneSchemaWith(schema, seen)
		}
	}
	if document.Paths != nil {
		clone.Paths = make(map[string]PathItem, len(document.Paths))
		for path, item := range document.Paths {
			itemClone := make(PathItem, len(item))
			for method, operation := range item {
				itemClone[method] = cloneOperation(operation, seen)
			}
			clone.Paths[path] = itemClone
		}
	}
	return clone
}

func cloneOperation(operation *Operation, seen map[*Schema]*Schema) *Operation {
	if operation == nil {
		return nil
	}
	clone := &Operation{
		Summary: operation.Summary,
		Tags:    slices.Clone(operation.Tags),
	}
	if operation.Parameters != nil {
		clone.Parameters = make([]*Parameter, len(operation.Parameters))
		for i, parameter := range operation.Parameters {
			if parameter == nil {
				continue
			}
			copy := *parameter
			copy.Schema = cloneSchemaWith(parameter.Schema, seen)
			clone.Parameters[i] = &copy
		}
	}
	if operation.RequestBody != nil {
		clone.RequestBody = &RequestBody{
			Required: operation.RequestBody.Required,
			Content:  cloneContent(operation.RequestBody.Content, seen),
		}
	}
	if operation.Responses != nil {
		clone.Responses = make(map[string]*Response, len(operation.Responses))
		for code, response := range operation.Responses {
			if response == nil {
				clone.Responses[code] = nil
				continue
			}
			clone.Responses[code] = &Response{
				Description: response.Description,
				Content:     cloneContent(response.Content, seen),
			}
		}
	}
	return clone
}

func cloneContent(content map[string]*MediaType, seen map[*Schema]*Schema) map[string]*MediaType {
	if content == nil {
		return nil
	}
	clone := make(map[string]*MediaType, len(content))
	for name, mediaType := range content {
		if mediaType == nil {
			continue
		}
		clone[name] = &MediaType{Schema: cloneSchemaWith(mediaType.Schema, seen)}
	}
	return clone
}
