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

// TestReadOnlyMode checks the read-only server exposes nothing that sends
// traffic or changes the project: every listed tool carries the read-only
// annotation, the reported count is what is listed, and a withheld tool
// cannot be called by name.
func TestReadOnlyMode(t *testing.T) {
	var reported int
	env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
		n, err := tools.RegisterAllWith(context.Background(), s, c, tools.Options{ReadOnly: true})
		if err != nil {
			t.Fatalf("RegisterAllWith: %v", err)
		}
		reported = n
	})
	listed, err := env.MCPClient.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(listed.Tools) == 0 {
		t.Fatal("read-only mode exposes no tools at all")
	}
	if len(listed.Tools) != reported {
		t.Errorf("RegisterAllWith reported %d tools, %d are listed", reported, len(listed.Tools))
	}
	names := make(map[string]bool, len(listed.Tools))
	for _, tool := range listed.Tools {
		names[tool.Name] = true
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Errorf("%s is exposed in read-only mode without the read-only annotation", tool.Name)
		}
	}
	// One tool from each way of doing damage: sending traffic, deleting,
	// changing the project, and switching it.
	for _, name := range []string{
		"caido_send_request", "caido_batch_send", "caido_ws_connect",
		"caido_delete_project", "caido_update_scope", "caido_create_tamper_rule",
		"caido_select_project", "caido_forward_intercept",
	} {
		if names[name] {
			t.Errorf("%s must not be exposed in read-only mode", name)
		}
	}
	for _, name := range []string{"caido_list_requests", "caido_get_request", "caido_list_scopes"} {
		if !names[name] {
			t.Errorf("%s should stay available in read-only mode", name)
		}
	}

	// Not listed is not enough: the tool must be gone, not hidden.
	result, err := env.MCPClient.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "caido_send_request",
		Arguments: map[string]any{"raw": "GET / HTTP/1.1\r\nHost: a.example\r\n\r\n", "host": "a.example"},
	})
	if err == nil && !result.IsError {
		t.Fatal("caido_send_request was callable in read-only mode")
	}
	if vars := env.Mock.LastVariables("CreateReplaySession"); vars != nil {
		t.Fatal("a withheld tool reached Caido")
	}
}

// TestReadOnlyModeOffExposesEverything pins the default: without the
// option every tool is registered, including the mutating ones.
func TestReadOnlyModeOffExposesEverything(t *testing.T) {
	var all, ro int
	testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
		all, _ = tools.RegisterAllWith(context.Background(), s, c, tools.Options{})
	})
	testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
		ro, _ = tools.RegisterAllWith(context.Background(), s, c, tools.Options{ReadOnly: true})
	})
	if ro <= 0 || ro >= all {
		t.Fatalf("read-only exposes %d of %d tools; want a strict, non-empty subset", ro, all)
	}
}

// TestInstructionsReadOnly: the read-only instructions may only name tools
// that exist in read-only mode.
func TestInstructionsReadOnly(t *testing.T) {
	env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
		if _, err := tools.RegisterAllWith(context.Background(), s, c, tools.Options{ReadOnly: true}); err != nil {
			t.Fatalf("RegisterAllWith: %v", err)
		}
	})
	listed, err := env.MCPClient.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	registered := make(map[string]bool, len(listed.Tools))
	for _, tool := range listed.Tools {
		registered[tool.Name] = true
	}
	named := regexp.MustCompile(`caido_[a-z_]+`).FindAllString(tools.InstructionsReadOnly, -1)
	if len(named) < 5 {
		t.Fatalf("read-only instructions name %d tools", len(named))
	}
	for _, name := range named {
		if !registered[name] {
			t.Errorf("read-only instructions name %q, which is not available in read-only mode", name)
		}
	}
	if n := len(tools.InstructionsReadOnly); n > maxInstructionsBytes {
		t.Errorf("read-only instructions are %d bytes, over the %d byte budget", n, maxInstructionsBytes)
	}
	if !strings.Contains(tools.InstructionsReadOnly, "read-only") {
		t.Error("read-only instructions do not say the server is read-only")
	}
}
