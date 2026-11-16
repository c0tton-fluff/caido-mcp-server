package tools_test

import (
	"testing"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/testutil"
	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/tools"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestListPluginsReportsManifestAndBackend covers the package identity that
// makes a plugin callable: the manifestId to pass, and the backend's state.
// A frontend-only package must carry no backend.
func TestListPluginsReportsManifestAndBackend(t *testing.T) {
	env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
		tools.RegisterListPluginsTool(s, c)
	})
	env.Mock.On("PluginPackagesWithBackends", pluginPackagesFixture())

	result := env.CallTool(t, "caido_list_plugins", map[string]any{})
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
	out := testutil.UnmarshalToolResult[tools.ListPluginsOutput](t, result)
	if len(out.Packages) != 3 {
		t.Fatalf("packages = %d, want 3", len(out.Packages))
	}

	byManifest := map[string]tools.PluginSummary{}
	for _, p := range out.Packages {
		byManifest[p.ManifestID] = p
	}
	quickssrf, ok := byManifest["quickssrf"]
	if !ok {
		t.Fatalf("no package with manifestId quickssrf: %+v", out.Packages)
	}
	if quickssrf.Backend == nil {
		t.Fatal("quickssrf: backend missing")
	}
	if quickssrf.Backend.ID != "backend-1" || !quickssrf.Backend.Running {
		t.Errorf("quickssrf backend = %+v, want backend-1 running", quickssrf.Backend)
	}
	if ui, ok := byManifest["ui-only"]; !ok || ui.Backend != nil {
		t.Errorf("ui-only backend = %+v, want none", ui.Backend)
	}
}
