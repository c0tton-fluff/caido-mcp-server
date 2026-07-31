package tools_test

import (
	"testing"
)

// propEnum returns properties.<name>.enum as a []string.
func propEnum(t *testing.T, schema map[string]any, name string) ([]string, bool) {
	t.Helper()
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return nil, false
	}
	p, ok := props[name].(map[string]any)
	if !ok {
		return nil, false
	}
	raw, ok := p["enum"].([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		s, ok := v.(string)
		if !ok {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}

func eqStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestSchemaEnumsAdvertised asserts each closed-set param advertises exactly the
// value set its handler already accepts (handler default-rejects anything else,
// case-sensitively), so the enum adds no new rejection.
func TestSchemaEnumsAdvertised(t *testing.T) {
	sections := []string{
		"requestAll", "requestHeader", "requestBody", "requestPath", "requestQuery",
		"requestMethod", "requestFirstLine", "requestSNI", "responseAll",
		"responseHeader", "responseBody", "responseFirstLine", "responseStatusCode",
	}
	cases := []struct {
		tool, field string
		want        []string
	}{
		{"caido_automate_task_control", "action", []string{"start", "pause", "resume", "cancel"}},
		{"caido_intercept_control", "action", []string{"pause", "resume"}},
		{"caido_run_workflow", "type", []string{"active", "convert"}},
		{"caido_create_tamper_rule", "section", sections},
		{"caido_update_tamper_rule", "section", sections},
	}
	for _, c := range cases {
		schema := advertisedSchema(t, c.tool)
		got, ok := propEnum(t, schema, c.field)
		if !ok {
			t.Errorf("%s: %s has no enum", c.tool, c.field)
			continue
		}
		if !eqStrings(got, c.want) {
			t.Errorf("%s: %s enum = %v, want %v", c.tool, c.field, got, c.want)
		}
	}
}
