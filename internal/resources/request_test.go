package resources_test

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestReadRequestResourceRedactsSecrets proves the request resource never leaks
// Authorization/Cookie (request) or Set-Cookie (response) values to the LLM.
func TestReadRequestResourceRedactsSecrets(t *testing.T) {
	env := newResourceTestEnv(t)

	reqRaw := base64.StdEncoding.EncodeToString([]byte(
		"GET /admin HTTP/1.1\r\n" +
			"Host: example.com\r\n" +
			"Authorization: Bearer supersecrettoken\r\n" +
			"Cookie: session=secretcookievalue\r\n" +
			"\r\n",
	))
	respRaw := base64.StdEncoding.EncodeToString([]byte(
		"HTTP/1.1 200 OK\r\n" +
			"Content-Type: text/html\r\n" +
			"Set-Cookie: auth=supersecretsetcookie; HttpOnly\r\n" +
			"\r\n" +
			"<html>ok</html>",
	))

	env.Mock.On("GetRequest", map[string]any{
		"request": map[string]any{
			"id":        "req-secret",
			"method":    "GET",
			"host":      "example.com",
			"port":      443,
			"path":      "/admin",
			"query":     "",
			"isTls":     true,
			"createdAt": int64(1714900000000),
			"raw":       reqRaw,
			"response": map[string]any{
				"statusCode":    200,
				"roundtripTime": 55,
				"raw":           respRaw,
			},
		},
	})

	result, err := env.Client.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "caido://requests/req-secret",
	})
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if len(result.Contents) == 0 {
		t.Fatal("expected content")
	}
	text := result.Contents[0].Text

	for _, secret := range []string{
		"supersecrettoken",
		"secretcookievalue",
		"supersecretsetcookie",
	} {
		if strings.Contains(text, secret) {
			t.Errorf("resource output leaked secret %q\n---\n%s", secret, text)
		}
	}
	if !strings.Contains(text, "[REDACTED]") {
		t.Errorf("expected [REDACTED] in output, got:\n%s", text)
	}
	// Non-sensitive context must survive so the resource stays useful.
	if !strings.Contains(text, "Host: example.com") {
		t.Errorf("expected Host header preserved, got:\n%s", text)
	}
}
