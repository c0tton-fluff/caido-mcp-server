package testutil

import (
	"context"
	"encoding/json"
	"testing"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type MCPTestEnv struct {
	*TestEnv
	MCPServer *mcp.Server
	MCPClient *mcp.ClientSession
	cancel    context.CancelFunc
}

func NewMCPTestEnv(t *testing.T, register func(server *mcp.Server, client *caido.Client)) *MCPTestEnv {
	t.Helper()
	env := NewTestEnv(t)

	server := mcp.NewServer(
		&mcp.Implementation{Name: "test-server", Version: "0.0.1"},
		nil,
	)
	register(server, env.Client)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	go func() {
		_, _ = server.Connect(ctx, serverTransport, nil)
	}()

	mcpClient := mcp.NewClient(
		&mcp.Implementation{Name: "test-client", Version: "0.0.1"},
		nil,
	)
	session, err := mcpClient.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("mcp client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	return &MCPTestEnv{
		TestEnv:   env,
		MCPServer: server,
		MCPClient: session,
		cancel:    cancel,
	}
}

func (e *MCPTestEnv) CallTool(t *testing.T, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	result, err := e.MCPClient.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      name,
		Arguments: mustMarshalMap(args),
	})
	if err != nil {
		t.Fatalf("CallTool(%s) error: %v", name, err)
	}
	return result
}

func mustMarshalMap(m map[string]any) map[string]json.RawMessage {
	result := make(map[string]json.RawMessage, len(m))
	for k, v := range m {
		data, _ := json.Marshal(v)
		result[k] = json.RawMessage(data)
	}
	return result
}

func UnmarshalToolResult[T any](t *testing.T, result *mcp.CallToolResult) T {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("empty tool result content")
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	var out T
	if err := json.Unmarshal([]byte(text.Text), &out); err != nil {
		t.Fatalf("unmarshal tool result: %v\nraw: %s", err, text.Text)
	}
	return out
}
