package tools_test

import (
	"context"
	"testing"

	"github.com/c0tton-fluff/caido-mcp-server/internal/testutil"
	"github.com/c0tton-fluff/caido-mcp-server/internal/tools"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestAllToolsRegisterWithoutPanic(t *testing.T) {
	env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
		tools.RegisterAll(s, c)
	})

	result, err := env.MCPClient.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}

	for _, tool := range result.Tools {
		if tool.Description == "" {
			t.Errorf("tool %q has empty description", tool.Name)
		}
		if tool.InputSchema == nil {
			t.Errorf("tool %q has nil InputSchema", tool.Name)
		}
	}
}

func TestToolCount(t *testing.T) {
	env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
		tools.RegisterAll(s, c)
	})

	result, err := env.MCPClient.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}

	const expectedTools = 93
	if len(result.Tools) != expectedTools {
		t.Fatalf("want %d tools registered, got %d", expectedTools, len(result.Tools))
	}
}
