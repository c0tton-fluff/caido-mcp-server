package tools_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/testutil"
	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/tools"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// advertisedSchema lists tools through the MCP env and returns the named tool's
// InputSchema decoded into a generic map (the exact shape a client receives,
// after the normalizeToolSchemas middleware runs).
func advertisedSchema(t *testing.T, toolName string) map[string]any {
	t.Helper()
	env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
		tools.RegisterAll(s, c)
	})
	res, err := env.MCPClient.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}
	for _, tool := range res.Tools {
		if tool.Name != toolName {
			continue
		}
		raw, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatalf("marshal InputSchema: %v", err)
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("unmarshal InputSchema: %v", err)
		}
		return m
	}
	t.Fatalf("tool %q not found", toolName)
	return nil
}

// propNumber returns properties.<name>.<keyword> as a float64 (JSON numbers).
func propNumber(t *testing.T, schema map[string]any, name, keyword string) (float64, bool) {
	t.Helper()
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return 0, false
	}
	p, ok := props[name].(map[string]any)
	if !ok {
		return 0, false
	}
	v, ok := p[keyword].(float64)
	return v, ok
}

// TestSchemaConstraintsAdvertised asserts every mirrored bound is advertised on
// the right property, with the right keyword and value. These mirror handler
// checks that already reject the same inputs, so they add no new rejection.
func TestSchemaConstraintsAdvertised(t *testing.T) {
	const oneMB = 1 << 20
	cases := []struct {
		tool, field, keyword string
		want                 float64
	}{
		{"caido_create_tamper_rule", "name", "maxLength", 200},
		{"caido_create_tamper_rule", "condition", "maxLength", 10000},
		{"caido_update_tamper_rule", "name", "maxLength", 200},
		{"caido_update_tamper_rule", "condition", "maxLength", 10000},
		{"caido_create_scope", "name", "maxLength", 200},
		{"caido_create_scope", "allowlist", "maxItems", 100},
		{"caido_create_environment", "name", "maxLength", 200},
		{"caido_list_requests", "httpql", "maxLength", 10000},
		{"caido_list_intercept_entries", "filter", "maxLength", 10000},
		{"caido_convert_body", "body", "maxLength", oneMB},
		{"caido_batch_send", "requests", "maxItems", 50},
		{"caido_send_request", "raw", "maxLength", oneMB},
		{"caido_race_window_send", "host", "maxLength", 200},
		{"caido_race_window_send", "requests", "maxItems", 50},
		{"caido_race_window_send", "requests", "minItems", 1},
	}
	for _, c := range cases {
		schema := advertisedSchema(t, c.tool)
		if v, ok := propNumber(t, schema, c.field, c.keyword); !ok || v != c.want {
			t.Errorf("%s: %s.%s = %v (ok=%v), want %v", c.tool, c.field, c.keyword, v, ok, c.want)
		}
	}
}

// TestRaceWindowNestedRawConstraint asserts the per-item requests[].raw maxLength
// (a nested array-item schema) is advertised.
func TestRaceWindowNestedRawConstraint(t *testing.T) {
	schema := advertisedSchema(t, "caido_race_window_send")
	props, _ := schema["properties"].(map[string]any)
	reqs, _ := props["requests"].(map[string]any)
	items, _ := reqs["items"].(map[string]any)
	itemProps, _ := items["properties"].(map[string]any)
	raw, _ := itemProps["raw"].(map[string]any)
	if raw == nil {
		t.Fatalf("requests.items.properties.raw missing; got items=%v", items)
	}
	if v, ok := raw["maxLength"].(float64); !ok || v != (1<<20) {
		t.Errorf("requests[].raw.maxLength = %v (ok=%v), want %d", v, ok, 1<<20)
	}
}

func TestCreateFindingSchemaConstraints(t *testing.T) {
	schema := advertisedSchema(t, "caido_create_finding")

	// The mirrored maxLength bounds are advertised.
	if v, ok := propNumber(t, schema, "title", "maxLength"); !ok || v != 500 {
		t.Errorf("title.maxLength = %v (ok=%v), want 500", v, ok)
	}
	if v, ok := propNumber(t, schema, "description", "maxLength"); !ok || v != 10000 {
		t.Errorf("description.maxLength = %v (ok=%v), want 10000", v, ok)
	}

	// Inference is preserved: required still lists requestId and title, and the
	// unconstrained requestId property still exists (no schema regression).
	props, _ := schema["properties"].(map[string]any)
	if _, ok := props["requestId"]; !ok {
		t.Errorf("requestId property missing from schema")
	}
	req, _ := schema["required"].([]any)
	hasReq := map[string]bool{}
	for _, r := range req {
		if s, ok := r.(string); ok {
			hasReq[s] = true
		}
	}
	if !hasReq["requestId"] || !hasReq["title"] {
		t.Errorf("required = %v, want to include requestId and title", req)
	}
}
