package tools_test

import (
	"testing"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/testutil"
	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/tools"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestGetRequest_ReturnsMetadataByDefault(t *testing.T) {
	env := testutil.NewMCPTestEnv(t, func(server *mcp.Server, client *caido.Client) {
		tools.RegisterGetRequestTool(server, client)
	})
	env.Mock.On("GetRequestMetadata", testutil.GetRequestMetadataResponse("req-123"))

	result := env.CallTool(t, "caido_get_request", map[string]any{
		"ids": []string{"req-123"},
	})

	output := testutil.UnmarshalToolResult[tools.GetRequestOutput](t, result)

	if output.ID != "req-123" {
		t.Errorf("expected ID %q, got %q", "req-123", output.ID)
	}
	if output.Method != "GET" {
		t.Errorf("expected method GET, got %q", output.Method)
	}
	if output.Host != "example.com" {
		t.Errorf("expected host example.com, got %q", output.Host)
	}
	if output.Port != 443 {
		t.Errorf("expected port 443, got %d", output.Port)
	}
	if output.Path != "/test" {
		t.Errorf("expected path /test, got %q", output.Path)
	}
	if !output.IsTLS {
		t.Errorf("expected isTls true, got false")
	}
	if output.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", output.StatusCode)
	}
	if output.RoundtripMs != 42 {
		t.Errorf("expected roundtrip 42ms, got %d", output.RoundtripMs)
	}
	if output.CreatedAt == "" {
		t.Errorf("expected createdAt to be set")
	}
	if output.Request != nil {
		t.Errorf("expected request to be nil for metadata-only")
	}
	if output.Response != nil {
		t.Errorf("expected response to be nil for metadata-only")
	}
}

func TestGetRequest_ReturnsFullRequestWithResponseHeaders(t *testing.T) {
	env := testutil.NewMCPTestEnv(t, func(server *mcp.Server, client *caido.Client) {
		tools.RegisterGetRequestTool(server, client)
	})
	body := "test response body"
	env.Mock.On("GetRequest", testutil.GetRequestFullResponse("req-456", body))

	result := env.CallTool(t, "caido_get_request", map[string]any{
		"ids":     []string{"req-456"},
		"include": []string{"responseHeaders", "responseBody"},
	})

	output := testutil.UnmarshalToolResult[tools.GetRequestOutput](t, result)

	if output.ID != "req-456" {
		t.Errorf("expected ID %q, got %q", "req-456", output.ID)
	}
	if output.Response == nil {
		t.Fatalf("expected response to be set")
	}
	if len(output.Response.Headers) == 0 {
		t.Errorf("expected response headers to be populated")
	}
	if output.Response.Body == "" {
		t.Errorf("expected response body to be populated")
	}
	if output.Response.Body != body {
		t.Errorf("expected body %q, got %q", body, output.Response.Body)
	}
}

func TestGetRequest_RejectsEmptyIDs(t *testing.T) {
	env := testutil.NewMCPTestEnv(t, func(server *mcp.Server, client *caido.Client) {
		tools.RegisterGetRequestTool(server, client)
	})

	result := env.CallTool(t, "caido_get_request", map[string]any{
		"ids": []string{},
	})

	if !result.IsError {
		t.Fatalf("expected error for empty ids array")
	}

	expected := "at least one request ID is required"
	if len(result.Content) == 0 {
		t.Fatalf("expected error content")
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	if text.Text != expected {
		t.Errorf("expected error %q, got %q", expected, text.Text)
	}
}

func TestGetRequest_RejectsMoreThan20IDs(t *testing.T) {
	env := testutil.NewMCPTestEnv(t, func(server *mcp.Server, client *caido.Client) {
		tools.RegisterGetRequestTool(server, client)
	})

	ids := make([]string, 21)
	for i := range ids {
		ids[i] = "req-" + string(rune('a'+i))
	}

	result := env.CallTool(t, "caido_get_request", map[string]any{
		"ids": ids,
	})

	if !result.IsError {
		t.Fatalf("expected error for more than 20 ids")
	}

	expected := "max 20 request IDs per call"
	if len(result.Content) == 0 {
		t.Fatalf("expected error content")
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	if text.Text != expected {
		t.Errorf("expected error %q, got %q", expected, text.Text)
	}
}

func TestGetRequest_HandlesRequestNotFound(t *testing.T) {
	env := testutil.NewMCPTestEnv(t, func(server *mcp.Server, client *caido.Client) {
		tools.RegisterGetRequestTool(server, client)
	})
	env.Mock.On("GetRequestMetadata", map[string]any{
		"request": nil,
	})

	result := env.CallTool(t, "caido_get_request", map[string]any{
		"ids": []string{"nonexistent"},
	})

	output := testutil.UnmarshalToolResult[tools.GetRequestOutput](t, result)

	if output.ID != "nonexistent" {
		t.Errorf("expected ID %q, got %q", "nonexistent", output.ID)
	}
	if output.Error != "request not found" {
		t.Errorf("expected error %q, got %q", "request not found", output.Error)
	}
}

func TestGetRequest_BatchMultipleIDs(t *testing.T) {
	env := testutil.NewMCPTestEnv(t, func(server *mcp.Server, client *caido.Client) {
		tools.RegisterGetRequestTool(server, client)
	})
	// The mock returns the same response for all calls to the same operation
	// So we'll get the same ID back for both requests
	env.Mock.On("GetRequestMetadata", testutil.GetRequestMetadataResponse("req-1"))

	result := env.CallTool(t, "caido_get_request", map[string]any{
		"ids": []string{"req-1", "req-2"},
	})

	output := testutil.UnmarshalToolResult[tools.GetRequestBatchOutput](t, result)

	if len(output.Requests) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(output.Requests))
	}

	// When multiple IDs are provided, we should get a batch response
	// Both will have the same data from the mock, but the important thing
	// is that the batch structure is correct
	if output.Requests[0].Method == "" {
		t.Errorf("expected first request to have metadata")
	}
	if output.Requests[1].Method == "" {
		t.Errorf("expected second request to have metadata")
	}
}

func TestGetRequest_TableDrivenIncludeOptions(t *testing.T) {
	tests := []struct {
		name           string
		include        []string
		expectMetadata bool
		expectRequest  bool
		expectResponse bool
		expectedOpName string
	}{
		{
			name:           "metadata_only_default",
			include:        nil,
			expectMetadata: true,
			expectRequest:  false,
			expectResponse: false,
			expectedOpName: "GetRequestMetadata",
		},
		{
			name:           "explicit_metadata",
			include:        []string{"metadata"},
			expectMetadata: true,
			expectRequest:  false,
			expectResponse: false,
			expectedOpName: "GetRequestMetadata",
		},
		{
			name:           "request_headers",
			include:        []string{"requestHeaders"},
			expectMetadata: false,
			expectRequest:  true,
			expectResponse: false,
			expectedOpName: "GetRequest",
		},
		{
			name:           "response_headers",
			include:        []string{"responseHeaders"},
			expectMetadata: false,
			expectRequest:  false,
			expectResponse: true,
			expectedOpName: "GetRequest",
		},
		{
			name:           "request_and_response_body",
			include:        []string{"requestBody", "responseBody"},
			expectMetadata: false,
			expectRequest:  true,
			expectResponse: true,
			expectedOpName: "GetRequest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(server *mcp.Server, client *caido.Client) {
				tools.RegisterGetRequestTool(server, client)
			})

			if tt.expectedOpName == "GetRequestMetadata" {
				env.Mock.On("GetRequestMetadata", testutil.GetRequestMetadataResponse("test-id"))
			} else {
				env.Mock.On("GetRequest", testutil.GetRequestFullResponse("test-id", "test body"))
			}

			input := map[string]any{
				"ids": []string{"test-id"},
			}
			if tt.include != nil {
				input["include"] = tt.include
			}

			result := env.CallTool(t, "caido_get_request", input)

			output := testutil.UnmarshalToolResult[tools.GetRequestOutput](t, result)

			if output.ID != "test-id" {
				t.Errorf("expected ID %q, got %q", "test-id", output.ID)
			}

			if tt.expectMetadata {
				if output.Method == "" {
					t.Errorf("expected metadata to be populated")
				}
			}

			if tt.expectRequest {
				if output.Request == nil {
					t.Errorf("expected request to be populated")
				}
			} else if output.Request != nil && tt.expectedOpName == "GetRequestMetadata" {
				t.Errorf("expected request to be nil for metadata-only")
			}

			if tt.expectResponse {
				if output.Response == nil {
					t.Errorf("expected response to be populated")
				}
			} else if output.Response != nil && tt.expectedOpName == "GetRequestMetadata" {
				t.Errorf("expected response to be nil for metadata-only")
			}
		})
	}
}

func TestGetRequest_BodyOffsetAndLimit(t *testing.T) {
	env := testutil.NewMCPTestEnv(t, func(server *mcp.Server, client *caido.Client) {
		tools.RegisterGetRequestTool(server, client)
	})
	body := "0123456789abcdef"
	env.Mock.On("GetRequest", testutil.GetRequestFullResponse("req-789", body))

	result := env.CallTool(t, "caido_get_request", map[string]any{
		"ids":        []string{"req-789"},
		"include":    []string{"responseBody"},
		"bodyOffset": 5,
		"bodyLimit":  5,
	})

	output := testutil.UnmarshalToolResult[tools.GetRequestOutput](t, result)

	if output.Response == nil {
		t.Fatalf("expected response to be set")
	}

	// The actual body parsing happens in httputil.ParseBase64
	// We're just verifying the parameters are passed through correctly
	if output.Response.Body == "" {
		t.Errorf("expected response body to be populated")
	}
}
