package tools_test

import (
	"testing"

	"github.com/c0tton-fluff/caido-mcp-server/internal/testutil"
	"github.com/c0tton-fluff/caido-mcp-server/internal/tools"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestCreateReplaySession(t *testing.T) {
	tests := []struct {
		name     string
		args     map[string]any
		setup    func(*testutil.MockHandler)
		wantID   string
		wantName string
		wantErr  bool
		// verifyVars asserts the GraphQL "variables" the handler transmitted.
		// It reads the input object off MockHandler.LastVariables so the checks
		// exercise the real wire payload, not just the returned output.
		verifyVars func(t *testing.T, input map[string]any)
	}{
		{
			name: "creates session with defaults",
			args: map[string]any{},
			setup: func(m *testutil.MockHandler) {
				m.On("CreateReplaySession", testutil.CreateReplaySessionResponse("sess-new"))
			},
			wantID: "sess-new",
			verifyVars: func(t *testing.T, input map[string]any) {
				assertKind(t, input, "HTTP")
			},
		},
		{
			name: "creates and renames session",
			args: map[string]any{"name": "auth-testing"},
			setup: func(m *testutil.MockHandler) {
				m.On("CreateReplaySession", testutil.CreateReplaySessionResponse("sess-named"))
				m.On("RenameReplaySession", testutil.RenameReplaySessionResponse("sess-named", "auth-testing"))
			},
			wantID:   "sess-named",
			wantName: "auth-testing",
			verifyVars: func(t *testing.T, input map[string]any) {
				assertKind(t, input, "HTTP")
			},
		},
		{
			name: "creates session with request source",
			args: map[string]any{"requestSourceId": "req-42"},
			setup: func(m *testutil.MockHandler) {
				m.On("CreateReplaySession", testutil.CreateReplaySessionResponse("sess-seeded"))
			},
			wantID: "sess-seeded",
			verifyVars: func(t *testing.T, input map[string]any) {
				assertKind(t, input, "HTTP")
				rs, ok := input["requestSource"].(map[string]any)
				if !ok {
					t.Fatalf("variables.input.requestSource not an object: %#v", input["requestSource"])
				}
				if got := rs["id"]; got != "req-42" {
					t.Fatalf("variables.input.requestSource.id = %#v, want %q", got, "req-42")
				}
			},
		},
		{
			name: "creates session in collection",
			args: map[string]any{"collectionId": "col-5", "name": "api-tests"},
			setup: func(m *testutil.MockHandler) {
				m.On("CreateReplaySession", testutil.CreateReplaySessionResponse("sess-col"))
				m.On("RenameReplaySession", testutil.RenameReplaySessionResponse("sess-col", "api-tests"))
			},
			wantID:   "sess-col",
			wantName: "api-tests",
			verifyVars: func(t *testing.T, input map[string]any) {
				assertKind(t, input, "HTTP")
				if got := input["collectionId"]; got != "col-5" {
					t.Fatalf("variables.input.collectionId = %#v, want %q", got, "col-5")
				}
			},
		},
		{
			name: "creates HTTP session via explicit kind",
			args: map[string]any{"kind": "HTTP"},
			setup: func(m *testutil.MockHandler) {
				m.On("CreateReplaySession", testutil.CreateReplaySessionResponse("sess-http"))
			},
			wantID: "sess-http",
			verifyVars: func(t *testing.T, input map[string]any) {
				assertKind(t, input, "HTTP")
			},
		},
		{
			name: "creates WS session via explicit kind",
			args: map[string]any{"kind": "WS"},
			setup: func(m *testutil.MockHandler) {
				m.On("CreateReplaySession", testutil.CreateReplaySessionResponse("sess-ws"))
			},
			wantID: "sess-ws",
			verifyVars: func(t *testing.T, input map[string]any) {
				assertKind(t, input, "WS")
			},
		},
		{
			name: "creates session with lowercase kind (normalized)",
			args: map[string]any{"kind": "ws"},
			setup: func(m *testutil.MockHandler) {
				m.On("CreateReplaySession", testutil.CreateReplaySessionResponse("sess-ws-lower"))
			},
			wantID: "sess-ws-lower",
			verifyVars: func(t *testing.T, input map[string]any) {
				assertKind(t, input, "WS")
			},
		},
		{
			name:    "rejects invalid kind value",
			args:    map[string]any{"kind": "FTP"},
			setup:   func(m *testutil.MockHandler) {},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterCreateReplaySessionTool(s, c)
			})
			tt.setup(env.Mock)

			result := env.CallTool(t, "caido_create_replay_session", tt.args)

			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error result")
				}
				return
			}

			output := testutil.UnmarshalToolResult[tools.CreateReplaySessionOutput](t, result)
			if output.ID != tt.wantID {
				t.Fatalf("want id %q, got %q", tt.wantID, output.ID)
			}
			if tt.wantName != "" && output.Name != tt.wantName {
				t.Fatalf("want name %q, got %q", tt.wantName, output.Name)
			}

			if tt.verifyVars != nil {
				tt.verifyVars(t, createReplaySessionInput(t, env.Mock))
			}
		})
	}
}

// createReplaySessionInput extracts the variables.input object that the handler
// sent for the CreateReplaySession GraphQL operation.
func createReplaySessionInput(t *testing.T, m *testutil.MockHandler) map[string]any {
	t.Helper()
	vars := m.LastVariables("CreateReplaySession")
	if vars == nil {
		t.Fatal("no variables recorded for CreateReplaySession operation")
	}
	input, ok := vars["input"].(map[string]any)
	if !ok {
		t.Fatalf("variables.input not an object: %#v", vars["input"])
	}
	return input
}

// assertKind fails unless the transmitted variables.input.kind equals want.
func assertKind(t *testing.T, input map[string]any, want string) {
	t.Helper()
	if got := input["kind"]; got != want {
		t.Fatalf("variables.input.kind = %#v, want %q", got, want)
	}
}
