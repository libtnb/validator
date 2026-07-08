package openapi

import (
	"encoding/json"
	"reflect"
	"strings"
	"time"
)

var (
	timeType      = reflect.TypeOf(time.Time{})
	marshalerType = reflect.TypeOf((*json.Marshaler)(nil)).Elem()
)

// schemaOf reflects t into a schema. Named struct types land in components
// and are returned as a $ref; everything else is inlined.
func (g *Generator) schemaOf(t reflect.Type) *Schema {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	if s, ok := g.override[t]; ok {
		clone := *s
		return &clone
	}
	if t == timeType {
		return &Schema{Type: "string", Format: "date-time"}
	}

	switch t.Kind() {
	case reflect.String:
		return &Schema{Type: "string"}
	case reflect.Bool:
		return &Schema{Type: "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &Schema{Type: "integer"}
	case reflect.Float32, reflect.Float64:
		return &Schema{Type: "number"}
	case reflect.Slice, reflect.Array:
		return &Schema{Type: "array", Items: g.schemaOf(t.Elem())}
	case reflect.Map:
		return &Schema{Type: "object", AdditionalProperties: g.schemaOf(t.Elem())}
	case reflect.Struct:
		// a struct that marshals itself (carbon.DateTime, ...) is a string
		// on the wire unless overridden via WithType
		if t.Implements(marshalerType) || reflect.PointerTo(t).Implements(marshalerType) {
			return &Schema{Type: "string"}
		}
		return g.structRef(t)
	default:
		return &Schema{} // interfaces, funcs: anything goes
	}
}

// structRef registers t under components once and returns a reference;
// anonymous structs are inlined instead.
func (g *Generator) structRef(t reflect.Type) *Schema {
	if t.Name() == "" {
		return g.structSchema(t)
	}

	name, ok := g.named[t]
	if !ok {
		name = componentName(t)
		g.named[t] = name
		g.doc.Components.Schemas[name] = &Schema{} // placeholder breaks cycles
		g.doc.Components.Schemas[name] = g.structSchema(t)
	}

	return &Schema{Ref: "#/components/schemas/" + name}
}

// structSchema builds the object schema for t's json-visible fields, with
// validate-tag constraints applied.
func (g *Generator) structSchema(t reflect.Type) *Schema {
	s := &Schema{Type: "object", Properties: map[string]*Schema{}}
	rules := g.rulesFor(t)

	for i := range t.NumField() {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		name := jsonName(field)
		if name == "" {
			continue
		}

		prop := g.schemaOf(field.Type)
		if fr, ok := rules[indexKey(field.Index)]; ok {
			if applyRules(prop, fr.Rules, field.Type) {
				s.Required = append(s.Required, name)
			}
			if len(fr.Element) > 0 && prop.Items != nil {
				applyRules(prop.Items, fr.Element, field.Type.Elem())
			}
		}
		s.Properties[name] = prop
	}

	return s
}

// jsonName returns the wire name of a field, or "" when it is hidden.
func jsonName(field reflect.StructField) string {
	tag := field.Tag.Get("json")
	if tag == "-" {
		return ""
	}
	if name, _, _ := strings.Cut(tag, ","); name != "" {
		return name
	}
	if tag == "" && field.Tag.Get("uri") != "" && field.Tag.Get("form") == "" {
		return "" // uri-only fields are path parameters, not body properties
	}
	return field.Name
}

// componentName derives a readable, unique component name from a type,
// flattening generic instantiations: Page[biz.User] -> PageUser.
func componentName(t reflect.Type) string {
	name := t.Name()
	if i := strings.IndexByte(name, '['); i >= 0 {
		base := name[:i]
		args := name[i+1 : len(name)-1]
		var parts []string
		for arg := range strings.SplitSeq(args, ",") {
			if j := strings.LastIndexByte(arg, '.'); j >= 0 {
				arg = arg[j+1:]
			}
			parts = append(parts, strings.TrimSpace(arg))
		}
		name = base + strings.Join(parts, "")
	}
	return name
}
