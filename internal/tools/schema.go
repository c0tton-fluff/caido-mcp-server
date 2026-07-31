package tools

import "github.com/google/jsonschema-go/jsonschema"

// schemaFor builds the reflected JSON Schema for input type T (identical to what
// mcp.AddTool would infer from T's struct tags) and applies patch, returning it
// for use as mcp.Tool.InputSchema. Setting an explicit InputSchema tells the SDK
// to use this schema instead of re-inferring, so patch can add constraints
// (maxLength, maximum, enum, ...) that Go struct tags cannot express.
//
// It panics on reflection error: that is a programming error (an unsupported
// input type), caught deterministically at server startup rather than at call
// time.
func schemaFor[T any](patch func(*jsonschema.Schema)) *jsonschema.Schema {
	s, err := jsonschema.For[T](nil)
	if err != nil {
		panic(err)
	}
	patch(s)
	return s
}

// prop returns the sub-schema for a named property, or nil if absent. Using it
// keeps patch closures nil-safe if a field is ever renamed.
func prop(s *jsonschema.Schema, name string) *jsonschema.Schema {
	if s == nil || s.Properties == nil {
		return nil
	}
	return s.Properties[name]
}

func intPtr(i int) *int { return &i }
