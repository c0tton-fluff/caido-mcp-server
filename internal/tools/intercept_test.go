package tools_test

import (
	"context"
	"testing"

	"github.com/c0tton-fluff/caido-mcp-server/internal/testutil"
	"github.com/c0tton-fluff/caido-mcp-server/internal/tools"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// expectMissingRequiredRejected asserts the MCP layer rejects a call that omits a
// required field. The call is made with no arguments to trigger the missing-required
// path. A rejection may surface either as a CallTool transport error or as a tool
// result with IsError set: go-sdk v1.6+ returns tool/validation errors as an IsError
// result (per the MCP spec) rather than a Go error, so accept both forms.
func expectMissingRequiredRejected(t *testing.T, env *testutil.MCPTestEnv, name string) {
	t.Helper()
	res, err := env.MCPClient.CallTool(context.Background(), &mcp.CallToolParams{
		Name: name,
	})
	if err != nil {
		return
	}
	if res == nil || !res.IsError {
		t.Fatalf("CallTool(%s) expected rejection for missing required field", name)
	}
}

// TestInterceptStatusTool covers caido_intercept_status.
// SDK op: GetInterceptStatus -> data.interceptStatus (enum string RUNNING|PAUSED).
func TestInterceptStatusTool(t *testing.T) {
	tests := []struct {
		name       string
		mockData   map[string]any
		mockErr    bool
		wantErr    bool
		wantStatus string
	}{
		{
			name:       "running",
			mockData:   map[string]any{"interceptStatus": "RUNNING"},
			wantStatus: "RUNNING",
		},
		{
			name:       "paused",
			mockData:   map[string]any{"interceptStatus": "PAUSED"},
			wantStatus: "PAUSED",
		},
		{
			name:    "graphql error",
			mockErr: true,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterInterceptStatusTool(s, c)
			})
			if !tt.mockErr {
				env.Mock.On("GetInterceptStatus", tt.mockData)
			}

			result := env.CallTool(t, "caido_intercept_status", map[string]any{})
			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}
			out := testutil.UnmarshalToolResult[tools.InterceptStatusOutput](t, result)
			if out.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q", out.Status, tt.wantStatus)
			}
		})
	}
}

// TestInterceptControlTool covers caido_intercept_control.
// SDK ops: PauseIntercept -> data.pauseIntercept{status}, ResumeIntercept -> data.resumeIntercept{status}.
// The handler returns a hardcoded status per action; it does not read the payload status.
func TestInterceptControlTool(t *testing.T) {
	tests := []struct {
		name       string
		input      map[string]any
		mockOp     string
		mockData   map[string]any
		wantErr    bool
		wantAction string
		wantStatus string
	}{
		{
			name:       "pause",
			input:      map[string]any{"action": "pause"},
			mockOp:     "PauseIntercept",
			mockData:   map[string]any{"pauseIntercept": map[string]any{"status": "PAUSED"}},
			wantAction: "pause",
			wantStatus: "PAUSED",
		},
		{
			name:       "resume",
			input:      map[string]any{"action": "resume"},
			mockOp:     "ResumeIntercept",
			mockData:   map[string]any{"resumeIntercept": map[string]any{"status": "RUNNING"}},
			wantAction: "resume",
			wantStatus: "RUNNING",
		},
		{
			name:    "invalid action",
			input:   map[string]any{"action": "bogus"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterInterceptControlTool(s, c)
			})
			if tt.mockOp != "" {
				env.Mock.On(tt.mockOp, tt.mockData)
			}

			result := env.CallTool(t, "caido_intercept_control", tt.input)
			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}
			out := testutil.UnmarshalToolResult[tools.InterceptControlOutput](t, result)
			if out.Action != tt.wantAction {
				t.Errorf("action = %q, want %q", out.Action, tt.wantAction)
			}
			if out.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q", out.Status, tt.wantStatus)
			}
		})
	}

	t.Run("missing required action", func(t *testing.T) {
		env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
			tools.RegisterInterceptControlTool(s, c)
		})
		expectMissingRequiredRejected(t, env, "caido_intercept_control")
	})
}

// TestListInterceptEntriesTool covers caido_list_intercept_entries.
// SDK op: ListInterceptEntries -> data.interceptEntries{ edges[].node{id,request{...}}, pageInfo, count{value} }.
func TestListInterceptEntriesTool(t *testing.T) {
	overLong := make([]byte, 10001)
	for i := range overLong {
		overLong[i] = 'a'
	}

	successData := map[string]any{
		"interceptEntries": map[string]any{
			"edges": []map[string]any{
				{
					"cursor": "c1",
					"node": map[string]any{
						"id": "ie-1",
						"request": map[string]any{
							"id":        "req-1",
							"method":    "GET",
							"host":      "example.com",
							"port":      443,
							"path":      "/api",
							"query":     "q=1",
							"isTls":     true,
							"createdAt": int64(1714900000000),
							"length":    100,
							"response": map[string]any{
								"id":            "resp-1",
								"statusCode":    200,
								"roundtripTime": 12,
								"length":        50,
							},
						},
					},
				},
			},
			"pageInfo": map[string]any{
				"hasNextPage":     true,
				"hasPreviousPage": false,
				"startCursor":     nil,
				"endCursor":       "c1",
			},
			"count": map[string]any{"value": 1},
		},
	}

	tests := []struct {
		name        string
		input       map[string]any
		mockData    map[string]any
		mockErr     bool
		wantErr     bool
		wantCount   int
		wantTotal   int
		wantHasMore bool
		wantCursor  string
		wantMethod  string
		wantURL     string
		wantStatus  int
	}{
		{
			name:        "success with response",
			input:       map[string]any{"limit": 20},
			mockData:    successData,
			wantCount:   1,
			wantTotal:   1,
			wantHasMore: true,
			wantCursor:  "c1",
			wantMethod:  "GET",
			wantURL:     "https://example.com/api?q=1",
			wantStatus:  200,
		},
		{
			name:  "empty list",
			input: map[string]any{},
			mockData: map[string]any{
				"interceptEntries": map[string]any{
					"edges": []map[string]any{},
					"pageInfo": map[string]any{
						"hasNextPage":     false,
						"hasPreviousPage": false,
						"startCursor":     nil,
						"endCursor":       nil,
					},
					"count": map[string]any{"value": 0},
				},
			},
			wantCount: 0,
			wantTotal: 0,
		},
		{
			name:    "filter over max length",
			input:   map[string]any{"filter": string(overLong)},
			wantErr: true,
		},
		{
			name:    "graphql error",
			input:   map[string]any{},
			mockErr: true,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterListInterceptEntriesTool(s, c)
			})
			if tt.mockData != nil {
				env.Mock.On("ListInterceptEntries", tt.mockData)
			}

			result := env.CallTool(t, "caido_list_intercept_entries", tt.input)
			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}
			out := testutil.UnmarshalToolResult[tools.ListInterceptEntriesOutput](t, result)
			if len(out.Entries) != tt.wantCount {
				t.Fatalf("entries count = %d, want %d", len(out.Entries), tt.wantCount)
			}
			if out.Total != tt.wantTotal {
				t.Errorf("total = %d, want %d", out.Total, tt.wantTotal)
			}
			if out.HasMore != tt.wantHasMore {
				t.Errorf("hasMore = %v, want %v", out.HasMore, tt.wantHasMore)
			}
			if out.NextCursor != tt.wantCursor {
				t.Errorf("nextCursor = %q, want %q", out.NextCursor, tt.wantCursor)
			}
			if tt.wantCount > 0 {
				e := out.Entries[0]
				if e.Method != tt.wantMethod {
					t.Errorf("method = %q, want %q", e.Method, tt.wantMethod)
				}
				if e.URL != tt.wantURL {
					t.Errorf("url = %q, want %q", e.URL, tt.wantURL)
				}
				if e.StatusCode != tt.wantStatus {
					t.Errorf("statusCode = %d, want %d", e.StatusCode, tt.wantStatus)
				}
			}
		})
	}
}

// TestForwardInterceptTool covers caido_forward_intercept.
// SDK op: ForwardInterceptMessage -> data.forwardInterceptMessage{forwardedId}.
func TestForwardInterceptTool(t *testing.T) {
	overLong := make([]byte, 1048577)
	for i := range overLong {
		overLong[i] = 'a'
	}

	tests := []struct {
		name    string
		input   map[string]any
		mockOp  bool
		mockID  string
		wantErr bool
		wantID  string
	}{
		{
			name:   "forward unmodified",
			input:  map[string]any{"id": "ie-1"},
			mockOp: true,
			mockID: "fwd-1",
			wantID: "fwd-1",
		},
		{
			name:   "forward modified raw",
			input:  map[string]any{"id": "ie-2", "raw": "R0VUIC8gSFRUUC8xLjE="},
			mockOp: true,
			mockID: "fwd-2",
			wantID: "fwd-2",
		},
		{
			name:    "raw over max length",
			input:   map[string]any{"id": "ie-3", "raw": string(overLong)},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterForwardInterceptTool(s, c)
			})
			if tt.mockOp {
				env.Mock.On("ForwardInterceptMessage", map[string]any{
					"forwardInterceptMessage": map[string]any{"forwardedId": tt.mockID},
				})
			}

			result := env.CallTool(t, "caido_forward_intercept", tt.input)
			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}
			out := testutil.UnmarshalToolResult[tools.ForwardInterceptOutput](t, result)
			if out.ForwardedID != tt.wantID {
				t.Errorf("forwardedId = %q, want %q", out.ForwardedID, tt.wantID)
			}
		})
	}

	t.Run("missing required id", func(t *testing.T) {
		env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
			tools.RegisterForwardInterceptTool(s, c)
		})
		expectMissingRequiredRejected(t, env, "caido_forward_intercept")
	})
}

// TestDropInterceptTool covers caido_drop_intercept.
// SDK op: DropInterceptMessage -> data.dropInterceptMessage{droppedId}.
func TestDropInterceptTool(t *testing.T) {
	tests := []struct {
		name    string
		input   map[string]any
		mockOp  bool
		mockID  string
		wantErr bool
		wantID  string
	}{
		{
			name:   "drop success",
			input:  map[string]any{"id": "ie-1"},
			mockOp: true,
			mockID: "drop-1",
			wantID: "drop-1",
		},
		{
			name:    "graphql error",
			input:   map[string]any{"id": "ie-x"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterDropInterceptTool(s, c)
			})
			if tt.mockOp {
				env.Mock.On("DropInterceptMessage", map[string]any{
					"dropInterceptMessage": map[string]any{"droppedId": tt.mockID},
				})
			}

			result := env.CallTool(t, "caido_drop_intercept", tt.input)
			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}
			out := testutil.UnmarshalToolResult[tools.DropInterceptOutput](t, result)
			if out.DroppedID != tt.wantID {
				t.Errorf("droppedId = %q, want %q", out.DroppedID, tt.wantID)
			}
		})
	}

	t.Run("missing required id", func(t *testing.T) {
		env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
			tools.RegisterDropInterceptTool(s, c)
		})
		expectMissingRequiredRejected(t, env, "caido_drop_intercept")
	})
}
