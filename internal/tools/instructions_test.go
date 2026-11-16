package tools_test

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/testutil"
	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/tools"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// maxInstructionsBytes bounds the server instructions: clients keep them in
// the model's context for the whole session, so every byte is paid per
// conversation.
const maxInstructionsBytes = 2500

// TestInstructions keeps the server instructions honest: every tool they
// name must be registered (a renamed or removed tool would otherwise leave
// agents calling something that does not exist), and they stay small.
func TestInstructions(t *testing.T) {
	env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
		tools.RegisterAll(s, c)
	})
	listed, err := env.MCPClient.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	registered := make(map[string]bool, len(listed.Tools))
	for _, tool := range listed.Tools {
		registered[tool.Name] = true
	}

	named := regexp.MustCompile(`caido_[a-z_]+`).FindAllString(tools.Instructions, -1)
	if len(named) < 5 {
		t.Fatalf("instructions name %d tools, want the workflow spelled out", len(named))
	}
	for _, name := range named {
		if !registered[name] {
			t.Errorf("instructions name %q, which is not a registered tool", name)
		}
	}

	if n := len(tools.Instructions); n > maxInstructionsBytes {
		t.Errorf("instructions are %d bytes, over the %d byte budget", n, maxInstructionsBytes)
	}
	// The contract this text exists to teach.
	for _, must := range []string{"[REDACTED]", "sessionId", "cookie jar"} {
		if !strings.Contains(tools.Instructions, must) {
			t.Errorf("instructions no longer mention %q", must)
		}
	}
}
