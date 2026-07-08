package resources

import (
	"context"
	"strings"
	"testing"

	"github.com/c0tton-fluff/caido-mcp-server/internal/testutil"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// newScopesTestEnv wires only the scopes resource, independent of
// RegisterAll, so this test does not depend on resources.go being updated
// to register it.
func newScopesTestEnv(t *testing.T) (*testutil.TestEnv, *mcp.ClientSession) {
	t.Helper()
	env := testutil.NewTestEnv(t)

	server := mcp.NewServer(
		&mcp.Implementation{Name: "test-server", Version: "0.0.1"},
		nil,
	)
	registerScopesResource(server, env.Client)

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
	t.Cleanup(func() { _ = session.Close() })

	return env, session
}

func TestReadScopesResource(t *testing.T) {
	env, client := newScopesTestEnv(t)
	env.Mock.On("ListScopes", map[string]any{
		"scopes": []map[string]any{
			{
				"id":        "s1",
				"name":      "Prod",
				"allowlist": []string{"example.com"},
				"denylist":  []string{"admin.example.com"},
				"indexed":   true,
			},
		},
	})

	result, err := client.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "caido://scopes",
	})
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if len(result.Contents) == 0 {
		t.Fatal("expected content")
	}
	text := result.Contents[0].Text
	if !strings.Contains(text, "Prod") {
		t.Errorf("expected scope name in output, got: %s", text)
	}
	if !strings.Contains(text, "example.com") {
		t.Errorf("expected allowlist entry in output, got: %s", text)
	}
}

func TestReadScopesResourceEmpty(t *testing.T) {
	env, client := newScopesTestEnv(t)
	env.Mock.On("ListScopes", map[string]any{
		"scopes": []map[string]any{},
	})

	result, err := client.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "caido://scopes",
	})
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if len(result.Contents) == 0 {
		t.Fatal("expected content")
	}
	text := result.Contents[0].Text
	if !strings.Contains(text, "no scopes") {
		t.Errorf("expected empty-state message, got: %s", text)
	}
}
