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
	}{
		{
			name: "creates session with defaults",
			args: map[string]any{},
			setup: func(m *testutil.MockHandler) {
				m.On("CreateReplaySession", testutil.CreateReplaySessionResponse("sess-new"))
			},
			wantID: "sess-new",
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
		},
		{
			name: "creates session with request source",
			args: map[string]any{"requestSourceId": "req-42"},
			setup: func(m *testutil.MockHandler) {
				m.On("CreateReplaySession", testutil.CreateReplaySessionResponse("sess-seeded"))
			},
			wantID: "sess-seeded",
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
		},
		{
			name: "creates HTTP session via explicit kind",
			args: map[string]any{"kind": "HTTP"},
			setup: func(m *testutil.MockHandler) {
				m.On("CreateReplaySession", testutil.CreateReplaySessionResponse("sess-http"))
			},
			wantID: "sess-http",
		},
		{
			name: "creates WS session via explicit kind",
			args: map[string]any{"kind": "WS"},
			setup: func(m *testutil.MockHandler) {
				m.On("CreateReplaySession", testutil.CreateReplaySessionResponse("sess-ws"))
			},
			wantID: "sess-ws",
		},
		{
			name: "creates session with lowercase kind (normalized)",
			args: map[string]any{"kind": "ws"},
			setup: func(m *testutil.MockHandler) {
				m.On("CreateReplaySession", testutil.CreateReplaySessionResponse("sess-ws-lower"))
			},
			wantID: "sess-ws-lower",
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
		})
	}
}
