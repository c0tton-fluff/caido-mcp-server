package tools_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/c0tton-fluff/caido-mcp-server/internal/replay"
	"github.com/c0tton-fluff/caido-mcp-server/internal/tools"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// liveMCPEnv wires a tool over an in-memory MCP transport using a caller-
// supplied (real) Caido client, for live integration tests.
type liveMCPEnv struct {
	session *mcp.ClientSession
}

func newLiveMCPEnv(
	t *testing.T, client *caido.Client,
	register func(*mcp.Server, *caido.Client),
) *liveMCPEnv {
	t.Helper()
	server := mcp.NewServer(
		&mcp.Implementation{Name: "live-test", Version: "0.0.1"}, nil,
	)
	register(server, client)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	go func() { _, _ = server.Connect(ctx, serverTransport, nil) }()

	mcpClient := mcp.NewClient(
		&mcp.Implementation{Name: "live-client", Version: "0.0.1"}, nil,
	)
	session, err := mcpClient.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("mcp connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return &liveMCPEnv{session: session}
}

func (e *liveMCPEnv) call(
	t *testing.T, name string, args map[string]any,
) *mcp.CallToolResult {
	t.Helper()
	raw := make(map[string]json.RawMessage, len(args))
	for k, v := range args {
		data, _ := json.Marshal(v)
		raw[k] = data
	}
	result, err := e.session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      name,
		Arguments: raw,
	})
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
		}
	}
	return result
}

func textOf(result *mcp.CallToolResult) string {
	if len(result.Content) == 0 {
		return ""
	}
	if tc, ok := result.Content[0].(*mcp.TextContent); ok {
		return tc.Text
	}
	return ""
}

func unmarshalLive[T any](t *testing.T, result *mcp.CallToolResult) T {
	t.Helper()
	var out T
	if err := json.Unmarshal([]byte(textOf(result)), &out); err != nil {
		t.Fatalf("unmarshal result: %v\nraw: %s", err, textOf(result))
	}
	return out
}

// TestSendRequestLive drives the caido_send_request tool through the full
// MCP transport against a real Caido 0.57 instance. Skipped unless
// CAIDO_IT_URL is set (e.g. http://localhost:8457 for a dockerized
// caido/caido:0.57.0 started with --allow-guests).
func TestSendRequestLive(t *testing.T) {
	url := os.Getenv("CAIDO_IT_URL")
	if url == "" {
		t.Skip("set CAIDO_IT_URL to run live send_request test")
	}
	replay.ResetDefaultSession("")
	t.Cleanup(func() { replay.ResetDefaultSession("") })

	target := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTeapot)
			_, _ = w.Write([]byte("live-mcp-ok"))
		},
	))
	defer target.Close()

	targetHost := os.Getenv("CAIDO_IT_TARGET_HOST")
	if targetHost == "" {
		targetHost = "host.docker.internal"
	}
	_, portStr, _ := strings.Cut(strings.TrimPrefix(target.URL, "http://"), ":")
	port, _ := strconv.Atoi(portStr)

	ctx := context.Background()
	client, err := caido.NewClient(caido.Options{URL: url})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	// Guest auth + project selection.
	gl, err := client.Auth.LoginAsGuest(ctx)
	if err != nil {
		t.Fatalf("login as guest: %v", err)
	}
	if gl.LoginAsGuest.Token == nil {
		t.Fatalf("no guest token (instance needs --allow-guests)")
	}
	client.SetAccessToken(gl.LoginAsGuest.Token.AccessToken)
	projects, err := client.Projects.List(ctx)
	if err != nil {
		t.Fatalf("list projects: %v", err)
	}
	if len(projects.Projects) == 0 {
		t.Skip("no project on instance; create one first")
	}
	if _, err := client.Projects.Select(ctx, projects.Projects[0].Id); err != nil {
		t.Fatalf("select project: %v", err)
	}
	time.Sleep(2 * time.Second)

	// Wire the real tool through MCP and call it.
	env := newLiveMCPEnv(t, client, func(s *mcp.Server, c *caido.Client) {
		tools.RegisterSendRequestTool(s, c)
	})

	result := env.call(t, "caido_send_request", map[string]any{
		"raw":  "GET / HTTP/1.1\r\nHost: " + targetHost + ":" + portStr + "\r\nConnection: close\r\n\r\n",
		"host": targetHost,
		"port": port,
		"tls":  false,
	})
	if result.IsError {
		t.Fatalf("tool returned error: %s", textOf(result))
	}
	out := unmarshalLive[tools.SendRequestOutput](t, result)
	if out.StatusCode != http.StatusTeapot {
		t.Fatalf("want status %d, got %d (err=%q)", http.StatusTeapot, out.StatusCode, out.Error)
	}
	if out.Response == nil || !strings.Contains(out.Response.Body, "live-mcp-ok") {
		t.Fatalf("expected target body in response, got %+v", out.Response)
	}
	t.Logf("live MCP send OK: status=%d sessionId=%s", out.StatusCode, out.SessionID)
}
