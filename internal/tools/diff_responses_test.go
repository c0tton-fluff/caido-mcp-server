package tools_test

import (
	"testing"

	"github.com/c0tton-fluff/caido-mcp-server/internal/testutil"
	"github.com/c0tton-fluff/caido-mcp-server/internal/tools"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// getRequestResponse mirrors testutil.GetRequestFullResponse but allows a
// custom status code, which the shared fixture hardcodes to 200.
func getRequestResponse(id string, status int, body string) map[string]any {
	return map[string]any{
		"request": map[string]any{
			"id":        id,
			"method":    "POST",
			"host":      "example.com",
			"port":      443,
			"path":      "/submit",
			"query":     "",
			"isTls":     true,
			"createdAt": int64(1714900000000),
			"raw":       testutil.RawHTTPRequest("POST", "/submit", "example.com"),
			"response": map[string]any{
				"statusCode":    status,
				"roundtripTime": 55,
				"raw":           testutil.RawHTTPResponse(status, body),
			},
		},
	}
}

func TestDiffResponses_DetectsStatusAndSizeChange(t *testing.T) {
	env := testutil.NewMCPTestEnv(t, func(server *mcp.Server, client *caido.Client) {
		tools.RegisterDiffResponsesTool(server, client)
	})
	// MockHandler.On queues responses per operation name; each call to
	// GetRequest pops the next entry in order, so the first queued entry
	// answers the idA fetch and the second answers idB.
	env.Mock.On("GetRequest", getRequestResponse("req-a", 200, "short"))
	env.Mock.On("GetRequest", getRequestResponse("req-b", 404, "a much longer response body"))

	result := env.CallTool(t, "caido_diff_responses", map[string]any{
		"idA": "req-a",
		"idB": "req-b",
	})

	output := testutil.UnmarshalToolResult[tools.DiffResponsesOutput](t, result)

	if output.IDA != "req-a" || output.IDB != "req-b" {
		t.Errorf("expected ids req-a/req-b, got %q/%q", output.IDA, output.IDB)
	}
	if output.StatusA != 200 || output.StatusB != 404 {
		t.Errorf("expected statuses 200/404, got %d/%d", output.StatusA, output.StatusB)
	}
	if !output.StatusChanged {
		t.Errorf("expected statusChanged true")
	}
	wantDelta := len("a much longer response body") - len("short")
	if output.SizeDelta != wantDelta {
		t.Errorf("expected sizeDelta %d, got %d", wantDelta, output.SizeDelta)
	}
	if output.BodyIdentical {
		t.Errorf("expected bodyIdentical false")
	}
	if output.Summary == "" {
		t.Errorf("expected a non-empty summary")
	}
}

func TestDiffResponses_IdenticalResponsesNotChanged(t *testing.T) {
	env := testutil.NewMCPTestEnv(t, func(server *mcp.Server, client *caido.Client) {
		tools.RegisterDiffResponsesTool(server, client)
	})
	env.Mock.On("GetRequest", getRequestResponse("req-a", 200, "same body"))
	env.Mock.On("GetRequest", getRequestResponse("req-b", 200, "same body"))

	result := env.CallTool(t, "caido_diff_responses", map[string]any{
		"idA": "req-a",
		"idB": "req-b",
	})

	output := testutil.UnmarshalToolResult[tools.DiffResponsesOutput](t, result)

	if output.StatusChanged {
		t.Errorf("expected statusChanged false")
	}
	if output.SizeDelta != 0 {
		t.Errorf("expected sizeDelta 0, got %d", output.SizeDelta)
	}
	if !output.BodyIdentical {
		t.Errorf("expected bodyIdentical true")
	}
	if output.Summary != "identical" {
		t.Errorf("expected summary %q, got %q", "identical", output.Summary)
	}
}

func TestDiffResponses_RejectsMissingIDs(t *testing.T) {
	env := testutil.NewMCPTestEnv(t, func(server *mcp.Server, client *caido.Client) {
		tools.RegisterDiffResponsesTool(server, client)
	})

	result := env.CallTool(t, "caido_diff_responses", map[string]any{
		"idA": "req-a",
		"idB": "",
	})

	if !result.IsError {
		t.Fatalf("expected error for missing idB")
	}
}

func TestDiffResponses_HandlesRequestNotFound(t *testing.T) {
	env := testutil.NewMCPTestEnv(t, func(server *mcp.Server, client *caido.Client) {
		tools.RegisterDiffResponsesTool(server, client)
	})
	env.Mock.On("GetRequest", map[string]any{"request": nil})

	result := env.CallTool(t, "caido_diff_responses", map[string]any{
		"idA": "nonexistent",
		"idB": "req-b",
	})

	if !result.IsError {
		t.Fatalf("expected error when idA is not found")
	}
}
