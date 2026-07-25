package tools_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/testutil"
	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/tools"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// assertCallRejected calls a tool directly and fails unless the call is
// rejected, either as a transport error (jsonschema "required" violation that
// never reaches the handler) or as a tool result error (handler validation).
func assertCallRejected(t *testing.T, env *testutil.MCPTestEnv, name string, args map[string]any) {
	t.Helper()
	raw := make(map[string]json.RawMessage, len(args))
	for k, v := range args {
		data, _ := json.Marshal(v)
		raw[k] = json.RawMessage(data)
	}
	result, err := env.MCPClient.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      name,
		Arguments: raw,
	})
	if err != nil {
		return
	}
	if !result.IsError {
		t.Fatal("expected error, got success")
	}
}

// TestListAutomateSessionsTool covers caido_list_automate_sessions.
// SDK op: ListAutomateSessions -> data.automateSessions { edges [{ node { id, name, createdAt } }] }.
func TestListAutomateSessionsTool(t *testing.T) {
	tests := []struct {
		name      string
		mockData  map[string]any
		mockErr   bool
		wantErr   bool
		wantCount int
		wantID    string
		wantName  string
	}{
		{
			name: "success",
			mockData: map[string]any{
				"automateSessions": map[string]any{
					"edges": []map[string]any{
						{
							"cursor": "c1",
							"node": map[string]any{
								"id":        "sess-1",
								"name":      "Login fuzz",
								"createdAt": int64(1714900000000),
							},
						},
					},
				},
			},
			wantCount: 1,
			wantID:    "sess-1",
			wantName:  "Login fuzz",
		},
		{
			name: "empty list",
			mockData: map[string]any{
				"automateSessions": map[string]any{
					"edges": []map[string]any{},
				},
			},
			wantCount: 0,
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
				tools.RegisterListAutomateSessionsTool(s, c)
			})
			if !tt.mockErr {
				env.Mock.On("ListAutomateSessions", tt.mockData)
			}

			result := env.CallTool(t, "caido_list_automate_sessions", map[string]any{})
			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}
			out := testutil.UnmarshalToolResult[tools.ListAutomateSessionsOutput](t, result)
			if len(out.Sessions) != tt.wantCount {
				t.Fatalf("sessions count = %d, want %d", len(out.Sessions), tt.wantCount)
			}
			if tt.wantCount > 0 {
				if out.Sessions[0].ID != tt.wantID {
					t.Errorf("id = %q, want %q", out.Sessions[0].ID, tt.wantID)
				}
				if out.Sessions[0].Name != tt.wantName {
					t.Errorf("name = %q, want %q", out.Sessions[0].Name, tt.wantName)
				}
			}
		})
	}
}

// TestGetAutomateSessionTool covers caido_get_automate_session.
// SDK op: GetAutomateSession -> data.automateSession { id, name, connection, raw, createdAt, entries }.
func TestGetAutomateSessionTool(t *testing.T) {
	rawTemplate := base64.StdEncoding.EncodeToString([]byte("GET /login HTTP/1.1"))
	tests := []struct {
		name         string
		input        map[string]any
		mockData     map[string]any
		mockNil      bool
		mockErr      bool
		rejectCall   bool
		wantErr      bool
		wantID       string
		wantTemplate string
		wantEntries  int
	}{
		{
			name:  "success",
			input: map[string]any{"id": "sess-1"},
			mockData: map[string]any{
				"automateSession": map[string]any{
					"id":   "sess-1",
					"name": "Login fuzz",
					"connection": map[string]any{
						"host":  "example.com",
						"port":  443,
						"isTLS": true,
					},
					"raw":       rawTemplate,
					"createdAt": int64(1714900000000),
					"entries": []map[string]any{
						{
							"id":        "entry-1",
							"name":      "Set A",
							"createdAt": int64(1714900100000),
						},
					},
				},
			},
			wantID:       "sess-1",
			wantTemplate: "GET /login HTTP/1.1",
			wantEntries:  1,
		},
		{
			name:       "missing id rejected",
			input:      map[string]any{},
			rejectCall: true,
		},
		{
			name:    "session not found",
			input:   map[string]any{"id": "sess-x"},
			mockNil: true,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterGetAutomateSessionTool(s, c)
			})
			if tt.rejectCall {
				assertCallRejected(t, env, "caido_get_automate_session", tt.input)
				return
			}
			if tt.mockNil {
				env.Mock.On("GetAutomateSession", map[string]any{"automateSession": nil})
			} else if !tt.mockErr {
				env.Mock.On("GetAutomateSession", tt.mockData)
			}

			result := env.CallTool(t, "caido_get_automate_session", tt.input)
			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}
			out := testutil.UnmarshalToolResult[tools.GetAutomateSessionOutput](t, result)
			if out.ID != tt.wantID {
				t.Errorf("id = %q, want %q", out.ID, tt.wantID)
			}
			if out.RequestTemplate != tt.wantTemplate {
				t.Errorf("requestTemplate = %q, want %q", out.RequestTemplate, tt.wantTemplate)
			}
			if len(out.Entries) != tt.wantEntries {
				t.Errorf("entries count = %d, want %d", len(out.Entries), tt.wantEntries)
			}
		})
	}
}

// TestGetAutomateEntryTool covers caido_get_automate_entry.
// SDK ops: GetAutomateEntry (metadata) then GetAutomateEntryRequests (results).
func TestGetAutomateEntryTool(t *testing.T) {
	payloadRaw := base64.StdEncoding.EncodeToString([]byte("admin"))
	entryMeta := map[string]any{
		"automateEntry": map[string]any{
			"id":        "entry-1",
			"name":      "Set A",
			"createdAt": int64(1714900100000),
			"connection": map[string]any{
				"host": "example.com", "port": 443, "isTLS": true,
			},
			"raw": "",
		},
	}
	entryRequests := map[string]any{
		"automateEntry": map[string]any{
			"id":   "entry-1",
			"name": "Set A",
			"requests": map[string]any{
				"edges": []map[string]any{
					{
						"cursor": "rc1",
						"node": map[string]any{
							"automateEntryId": "entry-1",
							"sequenceId":      "seq-1",
							"error":           nil,
							"payloads": []map[string]any{
								{"position": 0, "raw": payloadRaw},
							},
							"request": map[string]any{
								"id": "req-1", "host": "example.com", "port": 443,
								"path": "/login", "query": "", "method": "POST",
								"isTls": true, "length": 120,
								"createdAt": int64(1714900100000), "source": "AUTOMATE",
								"response": map[string]any{
									"id": "resp-1", "statusCode": 200,
									"roundtripTime": 42, "length": 512,
									"createdAt": int64(1714900100500),
								},
							},
						},
					},
				},
				"pageInfo": map[string]any{
					"hasNextPage": false, "hasPreviousPage": false,
					"startCursor": nil, "endCursor": nil,
				},
				"count": map[string]any{"value": 1},
			},
		},
	}

	tests := []struct {
		name        string
		input       map[string]any
		mockMeta    map[string]any
		mockReqs    map[string]any
		metaNil     bool
		rejectCall  bool
		wantErr     bool
		wantID      string
		wantResults int
		wantStatus  int
		wantPayload string
	}{
		{
			name:        "success",
			input:       map[string]any{"id": "entry-1"},
			mockMeta:    entryMeta,
			mockReqs:    entryRequests,
			wantID:      "entry-1",
			wantResults: 1,
			wantStatus:  200,
			wantPayload: "admin",
		},
		{
			name:       "missing id rejected",
			input:      map[string]any{},
			rejectCall: true,
		},
		{
			name:    "entry not found",
			input:   map[string]any{"id": "entry-x"},
			metaNil: true,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterGetAutomateEntryTool(s, c)
			})
			if tt.rejectCall {
				assertCallRejected(t, env, "caido_get_automate_entry", tt.input)
				return
			}
			if tt.metaNil {
				env.Mock.On("GetAutomateEntry", map[string]any{"automateEntry": nil})
			} else if tt.mockMeta != nil {
				env.Mock.On("GetAutomateEntry", tt.mockMeta)
				env.Mock.On("GetAutomateEntryRequests", tt.mockReqs)
			}

			result := env.CallTool(t, "caido_get_automate_entry", tt.input)
			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}
			out := testutil.UnmarshalToolResult[tools.GetAutomateEntryOutput](t, result)
			if out.ID != tt.wantID {
				t.Errorf("id = %q, want %q", out.ID, tt.wantID)
			}
			if len(out.Results) != tt.wantResults {
				t.Fatalf("results count = %d, want %d", len(out.Results), tt.wantResults)
			}
			if tt.wantResults > 0 {
				if out.Results[0].StatusCode != tt.wantStatus {
					t.Errorf("statusCode = %d, want %d", out.Results[0].StatusCode, tt.wantStatus)
				}
				if len(out.Results[0].Payloads) != 1 || out.Results[0].Payloads[0] != tt.wantPayload {
					t.Errorf("payloads = %v, want [%q]", out.Results[0].Payloads, tt.wantPayload)
				}
			}
		})
	}
}

// TestAutomateTaskControlTool covers caido_automate_task_control.
// SDK ops: StartAutomateTask, PauseAutomateTask, ResumeAutomateTask, CancelAutomateTask.
func TestAutomateTaskControlTool(t *testing.T) {
	taskPayload := map[string]any{"automateTask": map[string]any{"id": "task-1"}}
	tests := []struct {
		name       string
		input      map[string]any
		mockOp     string
		mockData   map[string]any
		wantErr    bool
		wantAction string
		wantTaskID string
	}{
		{
			name:       "start",
			input:      map[string]any{"action": "start", "session_id": "sess-1"},
			mockOp:     "StartAutomateTask",
			mockData:   map[string]any{"startAutomateTask": taskPayload},
			wantAction: "start",
			wantTaskID: "task-1",
		},
		{
			name:       "pause",
			input:      map[string]any{"action": "pause", "task_id": "task-1"},
			mockOp:     "PauseAutomateTask",
			mockData:   map[string]any{"pauseAutomateTask": taskPayload},
			wantAction: "pause",
			wantTaskID: "task-1",
		},
		{
			name:       "resume",
			input:      map[string]any{"action": "resume", "task_id": "task-1"},
			mockOp:     "ResumeAutomateTask",
			mockData:   map[string]any{"resumeAutomateTask": taskPayload},
			wantAction: "resume",
			wantTaskID: "task-1",
		},
		{
			name:   "cancel",
			input:  map[string]any{"action": "cancel", "task_id": "task-1"},
			mockOp: "CancelAutomateTask",
			mockData: map[string]any{
				"cancelAutomateTask": map[string]any{"cancelledId": "task-1"},
			},
			wantAction: "cancel",
			wantTaskID: "task-1",
		},
		{
			name:    "start missing session_id rejected",
			input:   map[string]any{"action": "start"},
			wantErr: true,
		},
		{
			name:    "pause missing task_id rejected",
			input:   map[string]any{"action": "pause"},
			wantErr: true,
		},
		{
			name:    "unknown action rejected",
			input:   map[string]any{"action": "frobnicate"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterAutomateTaskControlTool(s, c)
			})
			if tt.mockOp != "" {
				env.Mock.On(tt.mockOp, tt.mockData)
			}

			result := env.CallTool(t, "caido_automate_task_control", tt.input)
			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}
			out := testutil.UnmarshalToolResult[tools.AutomateTaskControlOutput](t, result)
			if out.Action != tt.wantAction {
				t.Errorf("action = %q, want %q", out.Action, tt.wantAction)
			}
			if out.TaskID != tt.wantTaskID {
				t.Errorf("taskId = %q, want %q", out.TaskID, tt.wantTaskID)
			}
		})
	}
}

// TestListTasksTool covers caido_list_tasks.
// SDK op: ListTasks -> data.tasks [{ __typename, id }] (Task oneof; __typename selects concrete type).
func TestListTasksTool(t *testing.T) {
	tests := []struct {
		name      string
		mockData  map[string]any
		mockErr   bool
		wantErr   bool
		wantCount int
		wantID    string
		wantType  string
	}{
		{
			name: "success",
			mockData: map[string]any{
				"tasks": []map[string]any{
					{"__typename": "ReplayTask", "id": "task-1"},
					{"__typename": "WorkflowTask", "id": "task-2"},
				},
			},
			wantCount: 2,
			wantID:    "task-1",
			wantType:  "ReplayTask",
		},
		{
			name:      "empty list",
			mockData:  map[string]any{"tasks": []map[string]any{}},
			wantCount: 0,
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
				tools.RegisterListTasksTool(s, c)
			})
			if !tt.mockErr {
				env.Mock.On("ListTasks", tt.mockData)
			}

			result := env.CallTool(t, "caido_list_tasks", map[string]any{})
			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}
			out := testutil.UnmarshalToolResult[tools.ListTasksOutput](t, result)
			if len(out.Tasks) != tt.wantCount {
				t.Fatalf("tasks count = %d, want %d", len(out.Tasks), tt.wantCount)
			}
			if tt.wantCount > 0 {
				if out.Tasks[0].ID != tt.wantID {
					t.Errorf("id = %q, want %q", out.Tasks[0].ID, tt.wantID)
				}
				if out.Tasks[0].Type != tt.wantType {
					t.Errorf("type = %q, want %q", out.Tasks[0].Type, tt.wantType)
				}
			}
		})
	}
}

// TestCancelTaskTool covers caido_cancel_task.
// SDK op: CancelTask -> data.cancelTask { cancelledId, error }.
func TestCancelTaskTool(t *testing.T) {
	tests := []struct {
		name       string
		input      map[string]any
		mockData   map[string]any
		mockErr    bool
		rejectCall bool
		wantErr    bool
		wantID     string
	}{
		{
			name:  "success",
			input: map[string]any{"id": "task-1"},
			mockData: map[string]any{
				"cancelTask": map[string]any{"cancelledId": "task-1", "error": nil},
			},
			wantID: "task-1",
		},
		{
			name:       "missing id rejected",
			input:      map[string]any{},
			rejectCall: true,
		},
		{
			name:    "graphql error",
			input:   map[string]any{"id": "task-1"},
			mockErr: true,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterCancelTaskTool(s, c)
			})
			if tt.rejectCall {
				assertCallRejected(t, env, "caido_cancel_task", tt.input)
				return
			}
			if !tt.mockErr && tt.mockData != nil {
				env.Mock.On("CancelTask", tt.mockData)
			}

			result := env.CallTool(t, "caido_cancel_task", tt.input)
			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}
			out := testutil.UnmarshalToolResult[tools.CancelTaskOutput](t, result)
			if out.ID != tt.wantID {
				t.Errorf("id = %q, want %q", out.ID, tt.wantID)
			}
			if !out.Cancelled {
				t.Errorf("cancelled = false, want true")
			}
		})
	}
}
