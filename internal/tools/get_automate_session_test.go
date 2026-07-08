package tools_test

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/c0tton-fluff/caido-mcp-server/internal/testutil"
	"github.com/c0tton-fluff/caido-mcp-server/internal/tools"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestGetAutomateSessionRedactsSecrets proves the fuzzing session's request
// template never leaks Authorization/Cookie values to the LLM.
func TestGetAutomateSessionRedactsSecrets(t *testing.T) {
	rawTemplate := base64.StdEncoding.EncodeToString([]byte(
		"POST /login HTTP/1.1\r\n" +
			"Host: example.com\r\n" +
			"Authorization: Bearer sessionsupersecret\r\n" +
			"Cookie: sid=sessionsecretcookie\r\n" +
			"\r\n" +
			"user=admin",
	))

	env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
		tools.RegisterGetAutomateSessionTool(s, c)
	})
	env.Mock.On("GetAutomateSession", map[string]any{
		"automateSession": map[string]any{
			"id":   "sess-1",
			"name": "Login fuzz",
			"connection": map[string]any{
				"host": "example.com", "port": 443, "isTLS": true,
			},
			"raw":       rawTemplate,
			"createdAt": int64(1714900000000),
			"entries":   []map[string]any{},
		},
	})

	result := env.CallTool(t, "caido_get_automate_session", map[string]any{"id": "sess-1"})
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
	out := testutil.UnmarshalToolResult[tools.GetAutomateSessionOutput](t, result)

	for _, secret := range []string{"sessionsupersecret", "sessionsecretcookie"} {
		if strings.Contains(out.RequestTemplate, secret) {
			t.Errorf("requestTemplate leaked secret %q: %s", secret, out.RequestTemplate)
		}
	}
	if !strings.Contains(out.RequestTemplate, "[REDACTED]") {
		t.Errorf("expected [REDACTED] in requestTemplate, got: %s", out.RequestTemplate)
	}
	// Body and non-sensitive headers must survive.
	if !strings.Contains(out.RequestTemplate, "user=admin") {
		t.Errorf("expected body preserved, got: %s", out.RequestTemplate)
	}
}
