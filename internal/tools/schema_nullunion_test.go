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

// TestNoNullUnionTypes guards the fix in schema_middleware.go (see
// normalizeToolSchemas / stripNullUnions for why ["null", <type>] unions break
// clients -- observed truncating run_workflow's *string "input"). It lists
// tools through a real client -- so the advertised, middleware-normalized schema
// is what gets inspected -- and walks each tool's FULL input schema tree (nested
// object properties, array items, $defs, and pointer fields, not just top-level
// params), failing if any parameter type is left as a ["null", ...] union.
func TestNoNullUnionTypes(t *testing.T) {
	env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
		tools.RegisterAll(s, c)
	})

	result, err := env.MCPClient.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}

	for _, tool := range result.Tools {
		raw, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatalf("tool %q: marshal input schema: %v", tool.Name, err)
		}
		var schema map[string]any
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatalf("tool %q: unmarshal input schema: %v", tool.Name, err)
		}
		walkSchemaTypes(t, tool.Name, "", schema)
	}
}

// walkSchemaTypes recursively descends a decoded JSON schema, flagging any
// "type" whose value is a ["null", ...] union (the shape clients mis-serialize).
func walkSchemaTypes(t *testing.T, tool, path string, node any) {
	t.Helper()
	switch v := node.(type) {
	case map[string]any:
		if typ, ok := v["type"].([]any); ok {
			for _, m := range typ {
				if m == "null" {
					t.Errorf(
						"tool %q: schema at %q uses a null union type %v; "+
							"params must be a single type "+
							"(see stripNullUnions in schema_middleware.go)",
						tool, path, typ,
					)
					break
				}
			}
		}
		for key, child := range v {
			walkSchemaTypes(t, tool, path+"/"+key, child)
		}
	case []any:
		for _, child := range v {
			walkSchemaTypes(t, tool, path, child)
		}
	}
}
