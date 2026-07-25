package tools_test

import (
	"encoding/base64"
	"testing"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/testutil"
	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/tools"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestListWorkflows covers the caido_list_workflows tool.
// GraphQL op: ListWorkflows -> {"workflows": [...]}.
func TestListWorkflows(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(*testutil.MockHandler)
		wantCount int
		wantFirst tools.WorkflowSummary
		wantErr   bool
	}{
		{
			name: "returns workflows",
			setup: func(m *testutil.MockHandler) {
				m.On("ListWorkflows", map[string]any{
					"workflows": []map[string]any{
						{
							"id":        "wf-1",
							"name":      "Active Scanner",
							"kind":      "ACTIVE",
							"enabled":   true,
							"global":    false,
							"readOnly":  false,
							"createdAt": "2024-01-01T00:00:00Z",
							"updatedAt": "2024-01-02T00:00:00Z",
						},
						{
							"id":        "wf-2",
							"name":      "Converter",
							"kind":      "CONVERT",
							"enabled":   false,
							"global":    true,
							"readOnly":  true,
							"createdAt": "2024-01-03T00:00:00Z",
							"updatedAt": "2024-01-04T00:00:00Z",
						},
					},
				})
			},
			wantCount: 2,
			wantFirst: tools.WorkflowSummary{
				ID:      "wf-1",
				Name:    "Active Scanner",
				Kind:    "ACTIVE",
				Enabled: true,
			},
		},
		{
			name: "returns empty list",
			setup: func(m *testutil.MockHandler) {
				m.On("ListWorkflows", map[string]any{
					"workflows": []map[string]any{},
				})
			},
			wantCount: 0,
		},
		{
			name: "graphql error",
			setup: func(m *testutil.MockHandler) {
				// No mock enqueued -> mock returns a GraphQL error.
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterListWorkflowsTool(s, c)
			})
			tt.setup(env.Mock)

			result := env.CallTool(t, "caido_list_workflows", map[string]any{})

			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error result")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}

			out := testutil.UnmarshalToolResult[tools.ListWorkflowsOutput](t, result)
			if len(out.Workflows) != tt.wantCount {
				t.Fatalf("want %d workflows, got %d", tt.wantCount, len(out.Workflows))
			}
			if tt.wantCount > 0 && out.Workflows[0] != tt.wantFirst {
				t.Fatalf("want first %+v, got %+v", tt.wantFirst, out.Workflows[0])
			}
		})
	}
}

// TestRunWorkflow covers the caido_run_workflow tool (active + convert branches).
// GraphQL ops: RunActiveWorkflow -> {"runActiveWorkflow": {"task": {"id"}, "error"}};
// RunConvertWorkflow -> {"runConvertWorkflow": {"output", "error"}}.
func TestRunWorkflow(t *testing.T) {
	// convertOutput is the plaintext the handler should yield after base64-decode.
	const convertOutput = "transformed-body"
	encodedOutput := base64.StdEncoding.EncodeToString([]byte(convertOutput))

	reqID := "req-1"
	overLimit := make([]byte, 1048576+1)

	tests := []struct {
		name       string
		args       map[string]any
		setup      func(*testutil.MockHandler)
		wantTaskID string
		wantOutput string
		wantErr    bool
	}{
		{
			name: "active workflow returns task id",
			args: map[string]any{
				"id":         "wf-active",
				"type":       "active",
				"request_id": reqID,
			},
			setup: func(m *testutil.MockHandler) {
				m.On("RunActiveWorkflow", map[string]any{
					"runActiveWorkflow": map[string]any{
						"error": nil,
						"task":  map[string]any{"id": "task-99"},
					},
				})
			},
			wantTaskID: "task-99",
		},
		{
			name: "convert workflow returns decoded output",
			args: map[string]any{
				"id":    "wf-convert",
				"type":  "convert",
				"input": "raw-body",
			},
			setup: func(m *testutil.MockHandler) {
				m.On("RunConvertWorkflow", map[string]any{
					"runConvertWorkflow": map[string]any{
						"error":  nil,
						"output": encodedOutput,
					},
				})
			},
			wantOutput: convertOutput,
		},
		{
			name: "active workflow user error",
			args: map[string]any{
				"id":         "wf-active",
				"type":       "active",
				"request_id": reqID,
			},
			setup: func(m *testutil.MockHandler) {
				m.On("RunActiveWorkflow", map[string]any{
					"runActiveWorkflow": map[string]any{
						"error": map[string]any{"__typename": "UnknownIdUserError"},
						"task":  nil,
					},
				})
			},
			wantErr: true,
		},
		{
			name: "active workflow missing request_id",
			args: map[string]any{
				"id":   "wf-active",
				"type": "active",
			},
			setup:   func(m *testutil.MockHandler) {},
			wantErr: true,
		},
		{
			name: "convert workflow missing input",
			args: map[string]any{
				"id":   "wf-convert",
				"type": "convert",
			},
			setup:   func(m *testutil.MockHandler) {},
			wantErr: true,
		},
		{
			name: "convert workflow input over 1MB",
			args: map[string]any{
				"id":    "wf-convert",
				"type":  "convert",
				"input": string(overLimit),
			},
			setup:   func(m *testutil.MockHandler) {},
			wantErr: true,
		},
		{
			name: "invalid type",
			args: map[string]any{
				"id":   "wf-x",
				"type": "bogus",
			},
			setup:   func(m *testutil.MockHandler) {},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterRunWorkflowTool(s, c)
			})
			tt.setup(env.Mock)

			result := env.CallTool(t, "caido_run_workflow", tt.args)

			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error result")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}

			out := testutil.UnmarshalToolResult[tools.RunWorkflowOutput](t, result)
			if tt.wantTaskID != "" {
				if out.TaskID == nil || *out.TaskID != tt.wantTaskID {
					t.Fatalf("want task_id %q, got %v", tt.wantTaskID, out.TaskID)
				}
			}
			if tt.wantOutput != "" {
				if out.Output == nil || *out.Output != tt.wantOutput {
					t.Fatalf("want output %q, got %v", tt.wantOutput, out.Output)
				}
			}
		})
	}
}

// TestToggleWorkflow covers the caido_toggle_workflow tool.
// GraphQL op: ToggleWorkflow -> {"toggleWorkflow": {"workflow": {...}, "error"}}.
func TestToggleWorkflow(t *testing.T) {
	tests := []struct {
		name        string
		args        map[string]any
		setup       func(*testutil.MockHandler)
		wantEnabled bool
		wantName    string
		wantErr     bool
	}{
		{
			name: "enables workflow",
			args: map[string]any{"id": "wf-1", "enabled": true},
			setup: func(m *testutil.MockHandler) {
				m.On("ToggleWorkflow", map[string]any{
					"toggleWorkflow": map[string]any{
						"error": nil,
						"workflow": map[string]any{
							"id":      "wf-1",
							"name":    "Active Scanner",
							"kind":    "ACTIVE",
							"enabled": true,
						},
					},
				})
			},
			wantEnabled: true,
			wantName:    "Active Scanner",
		},
		{
			name: "user error",
			args: map[string]any{"id": "wf-bad", "enabled": false},
			setup: func(m *testutil.MockHandler) {
				m.On("ToggleWorkflow", map[string]any{
					"toggleWorkflow": map[string]any{
						"error":    map[string]any{"__typename": "UnknownIdUserError"},
						"workflow": nil,
					},
				})
			},
			wantErr: true,
		},
		{
			name: "missing required id",
			args: map[string]any{"enabled": true},
			setup: func(m *testutil.MockHandler) {
				// Validation rejects before any GraphQL call.
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterToggleWorkflowTool(s, c)
			})
			tt.setup(env.Mock)

			result := env.CallTool(t, "caido_toggle_workflow", tt.args)

			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error result")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}

			out := testutil.UnmarshalToolResult[tools.ToggleWorkflowOutput](t, result)
			if out.Enabled != tt.wantEnabled {
				t.Fatalf("want enabled %v, got %v", tt.wantEnabled, out.Enabled)
			}
			if tt.wantName != "" && out.Name != tt.wantName {
				t.Fatalf("want name %q, got %q", tt.wantName, out.Name)
			}
		})
	}
}
