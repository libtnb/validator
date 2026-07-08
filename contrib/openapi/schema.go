package openapi

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"time"
)

var (
	timeType       = reflect.TypeFor[time.Time]()
	rawMessageType = reflect.TypeFor[json.RawMessage]()
	marshalerType  = reflect.TypeFor[json.Marshaler]()
)

// derefType follows pointers with a bound so a recursive named pointer type
// (type P *P) cannot hang generation; mirrors the main module's deref bound.
// A type still a pointer after the bound falls to the open-schema default.
func derefType(t reflect.Type) reflect.Type {
	for range 32 {
		if t.Kind() != reflect.Pointer {
			return t
		}
		t = t.Elem()
	}
	return t
}

// schemaOf reflects t into a schema. Named struct types land in components
// and are returned as a $ref; everything else is inlined.
func (g *Generator) schemaOf(t reflect.Type) *Schema {
	t = derefType(t)

	if s, ok := g.override[t]; ok {
		clone := *s
		return &clone
	}
	if t == timeType {
		return &Schema{Type: "string", Format: "date-time"}
	}
	if t == rawMessageType {
		return &Schema{} // pass-through JSON: anything goes
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
	case reflect.Slice:
		// encoding/json serializes byte slices as base64 strings, not arrays
		if t.Elem().Kind() == reflect.Uint8 {
			return &Schema{Type: "string", ContentEncoding: "base64"}
		}
		return &Schema{Type: "array", Items: g.schemaOf(t.Elem())}
	case reflect.Array:
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
// anonymous structs are inlined instead. Distinct types that flatten to the
// same component name (a.User vs b.User) get numeric suffixes instead of
// silently overwriting each other.
func (g *Generator) structRef(t reflect.Type) *Schema {
	if t.Name() == "" {
		return g.structSchema(t)
	}

	name, ok := g.named[t]
	if !ok {
		base := componentName(t)
		name = base
		for i := 2; ; i++ {
			if _, taken := g.owner[name]; !taken {
				break
			}
			name = base + strconv.Itoa(i)
		}
		g.named[t] = name
		g.owner[name] = t
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

	for _, bf := range jsonFields(t, nil, map[reflect.Type]bool{}) {
		if bf.name == "" {
			continue
		}
		if _, taken := s.Properties[bf.name]; taken {
			continue // shallower field wins, like encoding/json
		}
		prop := g.schemaOf(bf.Type)
		if fr, ok := rules[indexKey(bf.index)]; ok {
			if applyRules(prop, fr.Rules) {
				s.Required = append(s.Required, bf.name)
			}
			applyElementRules(prop, fr.Element)
		}
		s.Properties[bf.name] = prop
	}

	return s
}

// bodyField is one json-visible field, with its full index path from the root
// type so validate rules resolve across embedded promotions.
type bodyField struct {
	reflect.StructField
	name  string
	index []int
}

// jsonFields flattens t the way encoding/json serializes it: anonymous
// embedded structs promote their fields into the parent object (the wire
// format is flat), while an embedded field carrying an explicit json name
// stays a nested property. Unexported embedded types still promote their
// exported fields, exactly like encoding/json. Name clashes resolve
// shallowest-first via the caller's Properties check.
func jsonFields(t reflect.Type, prefix []int, seen map[reflect.Type]bool) []bodyField {
	if seen[t] {
		return nil
	}
	seen[t] = true
	defer delete(seen, t)

	var out []bodyField
	for i := range t.NumField() {
		f := t.Field(i)
		idx := append(append([]int{}, prefix...), i)

		if f.Anonymous {
			ft := derefType(f.Type)
			tagName, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if ft.Kind() == reflect.Struct && tagName == "" && f.Tag.Get("json") != "-" {
				out = append(out, jsonFields(ft, idx, seen)...)
				continue
			}
		}
		if !f.IsExported() {
			continue
		}
		// name is "" for body-hidden fields (json:"-", uri-only); they stay in
		// the list so request() can still route them as parameters.
		out = append(out, bodyField{StructField: f, name: jsonName(f), index: idx})
	}
	return out
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

// componentName derives a readable component name from a type, flattening
// generic instantiations of any depth: Envelope[Page[biz.User]] -> EnvelopePageUser.
func componentName(t reflect.Type) string {
	segments := strings.FieldsFunc(t.Name(), func(r rune) bool {
		return r == '[' || r == ']' || r == ',' || r == '*' || r == ' '
	})
	var b strings.Builder
	for _, segment := range segments {
		if j := strings.LastIndexByte(segment, '.'); j >= 0 {
			segment = segment[j+1:]
		}
		b.WriteString(segment)
	}
	return b.String()
}
