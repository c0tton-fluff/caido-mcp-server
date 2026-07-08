package tools_test

import (
	"encoding/base64"
	"fmt"
	"testing"

	"github.com/c0tton-fluff/caido-mcp-server/internal/replay"
	"github.com/c0tton-fluff/caido-mcp-server/internal/testutil"
	"github.com/c0tton-fluff/caido-mcp-server/internal/tools"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// rawHTTPResponseWithHeaders builds a base64-encoded raw HTTP response with
// caller-supplied extra header lines, for exercising fingerprint-expansion
// fields (title, redirect target, cookie names) that testutil.RawHTTPResponse
// does not support customizing.
func rawHTTPResponseWithHeaders(status int, extraHeaders, body string) string {
	raw := fmt.Sprintf(
		"HTTP/1.1 %d OK\r\nContent-Type: text/html\r\nContent-Length: %d\r\n%s\r\n\r\n%s",
		status, len(body), extraHeaders, body,
	)
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

// entryResponseWithRaw mirrors testutil.GetReplayEntryResponse but accepts a
// caller-built raw response so tests can inject custom headers.
func entryResponseWithRaw(entryID, requestID, rawResponse string, statusCode int) map[string]any {
	return map[string]any{
		"replayEntry": map[string]any{
			"__typename": "ReplayEntryHttp",
			"id":         entryID,
			"raw":        testutil.RawHTTPRequest("GET", "/test", "example.com"),
			"error":      nil,
			"createdAt":  int64(1714900000000),
			"connection": map[string]any{
				"host":  "example.com",
				"port":  443,
				"isTLS": true,
			},
			"settings": map[string]any{
				"placeholders": []any{},
			},
			"request": map[string]any{
				"id":        requestID,
				"method":    "GET",
				"host":      "example.com",
				"port":      443,
				"path":      "/test",
				"query":     "",
				"isTls":     true,
				"raw":       testutil.RawHTTPRequest("GET", "/test", "example.com"),
				"createdAt": int64(1714900000000),
				"response": map[string]any{
					"id":            "resp-" + requestID,
					"statusCode":    statusCode,
					"roundtripTime": 100,
					"raw":           rawResponse,
					"length":        len(rawResponse),
				},
			},
		},
	}
}

// setupSendMocks wires the Caido 0.57 default-session send flow:
//  1. GetOrCreateSession creates an empty session (cached).
//  2. Send -> sendOnSession: GetReplaySession reports no active entry,
//     so Send falls back to creating a session seeded with the request.
//  3. StartReplayTask runs the seeded draft.
//  4. PollForEntry: GetReplaySession reports the new active entry, then
//     GetReplayEntry returns the response.
func setupSendMocks(m *testutil.MockHandler, sessionID, entryID, requestID string, statusCode int) {
	// Empty session created by GetOrCreateSession.
	m.On("CreateReplaySession", testutil.CreateReplaySessionResponse(sessionID))
	// sendOnSession sees no active entry -> fallback.
	m.On("GetReplaySession", testutil.GetReplaySessionResponse(sessionID, ""))
	// Fallback seeded session.
	m.On("CreateReplaySession", testutil.CreateReplaySessionSeededResponse(sessionID, entryID))
	m.On("StartReplayTask", testutil.StartReplayTaskResponse())
	// Poll: new active entry present, then fetch it.
	m.On("GetReplaySession", testutil.GetReplaySessionResponse(sessionID, entryID))
	m.On("GetReplayEntry", testutil.GetReplayEntryResponse(entryID, requestID, statusCode, "response body"))
}

func TestSendRequest(t *testing.T) {
	tests := []struct {
		name       string
		args       map[string]any
		setup      func(*testutil.MockHandler)
		wantStatus int
		wantError  bool
	}{
		{
			name: "sends request and returns response",
			args: map[string]any{
				"raw":  "GET /test HTTP/1.1\r\nHost: example.com\r\n\r\n",
				"host": "example.com",
			},
			setup: func(m *testutil.MockHandler) {
				setupSendMocks(m, "sess-1", "entry-1", "req-1", 200)
			},
			wantStatus: 200,
		},
		{
			name: "uses provided sessionId",
			args: map[string]any{
				"raw":       "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n",
				"host":      "example.com",
				"sessionId": "my-session",
			},
			setup: func(m *testutil.MockHandler) {
				// Provided session already has an active entry, so Send
				// updates its draft and starts the task (no new session).
				m.On("GetReplaySession", testutil.GetReplaySessionResponse("my-session", "prev-entry"))
				m.On("UpdateReplayEntryDraft", testutil.UpdateReplayEntryDraftResponse("prev-entry"))
				m.On("StartReplayTask", testutil.StartReplayTaskResponse())
				m.On("GetReplaySession", testutil.GetReplaySessionResponse("my-session", "entry-2"))
				m.On("GetReplayEntry", testutil.GetReplayEntryResponse("entry-2", "req-2", 301, ""))
			},
			wantStatus: 301,
		},
		{
			name:      "rejects empty raw",
			args:      map[string]any{"raw": ""},
			setup:     func(m *testutil.MockHandler) {},
			wantError: true,
		},
		{
			name:      "rejects raw over 1MB",
			args:      map[string]any{"raw": string(make([]byte, 1048577)), "host": "x.com"},
			setup:     func(m *testutil.MockHandler) {},
			wantError: true,
		},
		{
			name:      "rejects missing host",
			args:      map[string]any{"raw": "GET / HTTP/1.1\r\n\r\n"},
			setup:     func(m *testutil.MockHandler) {},
			wantError: true,
		},
		{
			name: "extracts host from Host header",
			args: map[string]any{
				"raw": "GET / HTTP/1.1\r\nHost: auto.example.com\r\n\r\n",
			},
			setup: func(m *testutil.MockHandler) {
				setupSendMocks(m, "sess-3", "entry-3", "req-3", 200)
			},
			wantStatus: 200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			replay.ResetDefaultSession("")
			t.Cleanup(func() { replay.ResetDefaultSession("") })

			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterSendRequestTool(s, c)
			})
			tt.setup(env.Mock)

			result := env.CallTool(t, "caido_send_request", tt.args)

			if tt.wantError {
				if !result.IsError {
					t.Fatal("expected error result")
				}
				return
			}

			output := testutil.UnmarshalToolResult[tools.SendRequestOutput](t, result)
			if output.StatusCode != tt.wantStatus {
				t.Fatalf("want status %d, got %d", tt.wantStatus, output.StatusCode)
			}
		})
	}
}

// TestSendRequestFingerprintExpansion covers the Chunk 4 additions: the
// enriched Fingerprint fields, includeBody gating, and marker/reflected.
func TestSendRequestFingerprintExpansion(t *testing.T) {
	rawResp := rawHTTPResponseWithHeaders(
		302,
		"Location: /dashboard\r\nSet-Cookie: session=abc123; Path=/\r\nSet-Cookie: csrftoken=xyz\r\n",
		"<html><head><title>Welcome</title></head><body>hello world</body></html>",
	)

	setup := func(m *testutil.MockHandler) {
		m.On("CreateReplaySession", testutil.CreateReplaySessionResponse("sess-fp"))
		m.On("GetReplaySession", testutil.GetReplaySessionResponse("sess-fp", ""))
		m.On("CreateReplaySession", testutil.CreateReplaySessionSeededResponse("sess-fp", "entry-fp"))
		m.On("StartReplayTask", testutil.StartReplayTaskResponse())
		m.On("GetReplaySession", testutil.GetReplaySessionResponse("sess-fp", "entry-fp"))
		m.On("GetReplayEntry", entryResponseWithRaw("entry-fp", "req-fp", rawResp, 302))
	}

	t.Run("populates enriched fingerprint fields", func(t *testing.T) {
		replay.ResetDefaultSession("")
		t.Cleanup(func() { replay.ResetDefaultSession("") })

		env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
			tools.RegisterSendRequestTool(s, c)
		})
		setup(env.Mock)

		result := env.CallTool(t, "caido_send_request", map[string]any{
			"raw":  "GET /test HTTP/1.1\r\nHost: example.com\r\n\r\n",
			"host": "example.com",
		})
		if result.IsError {
			t.Fatalf("unexpected error result: %+v", result)
		}
		output := testutil.UnmarshalToolResult[tools.SendRequestOutput](t, result)

		if output.Response == nil || output.Response.Fingerprint == nil {
			t.Fatalf("expected response fingerprint, got %+v", output.Response)
		}
		fp := output.Response.Fingerprint
		if fp.StatusCode != 302 {
			t.Fatalf("want fingerprint statusCode 302, got %d", fp.StatusCode)
		}
		if fp.Title != "Welcome" {
			t.Fatalf("want title %q, got %q", "Welcome", fp.Title)
		}
		if fp.RedirectTarget != "/dashboard" {
			t.Fatalf("want redirectTarget %q, got %q", "/dashboard", fp.RedirectTarget)
		}
		if len(fp.SetCookies) != 2 || fp.SetCookies[0] != "session" || fp.SetCookies[1] != "csrftoken" {
			t.Fatalf("want setCookies [session csrftoken], got %v", fp.SetCookies)
		}
		if fp.WordCount == 0 {
			t.Fatalf("want non-zero wordCount, got %d", fp.WordCount)
		}
		if output.Response.Body == "" {
			t.Fatalf("want body included by default (includeBody defaults true)")
		}
	})

	t.Run("includeBody false omits body but keeps fingerprint", func(t *testing.T) {
		replay.ResetDefaultSession("")
		t.Cleanup(func() { replay.ResetDefaultSession("") })

		env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
			tools.RegisterSendRequestTool(s, c)
		})
		setup(env.Mock)

		result := env.CallTool(t, "caido_send_request", map[string]any{
			"raw":         "GET /test HTTP/1.1\r\nHost: example.com\r\n\r\n",
			"host":        "example.com",
			"includeBody": false,
		})
		if result.IsError {
			t.Fatalf("unexpected error result: %+v", result)
		}
		output := testutil.UnmarshalToolResult[tools.SendRequestOutput](t, result)

		if output.Response == nil {
			t.Fatalf("expected response, got nil")
		}
		if output.Response.Body != "" {
			t.Fatalf("want empty body when includeBody:false, got %q", output.Response.Body)
		}
		if output.Response.Fingerprint == nil || output.Response.Fingerprint.Title != "Welcome" {
			t.Fatalf("want fingerprint retained when includeBody:false, got %+v", output.Response.Fingerprint)
		}
	})

	t.Run("marker sets reflected true when present", func(t *testing.T) {
		replay.ResetDefaultSession("")
		t.Cleanup(func() { replay.ResetDefaultSession("") })

		env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
			tools.RegisterSendRequestTool(s, c)
		})
		setup(env.Mock)

		result := env.CallTool(t, "caido_send_request", map[string]any{
			"raw":    "GET /test HTTP/1.1\r\nHost: example.com\r\n\r\n",
			"host":   "example.com",
			"marker": "hello world",
		})
		if result.IsError {
			t.Fatalf("unexpected error result: %+v", result)
		}
		output := testutil.UnmarshalToolResult[tools.SendRequestOutput](t, result)

		if output.Reflected == nil || !*output.Reflected {
			t.Fatalf("want reflected=true, got %+v", output.Reflected)
		}
	})

	t.Run("marker sets reflected false when absent", func(t *testing.T) {
		replay.ResetDefaultSession("")
		t.Cleanup(func() { replay.ResetDefaultSession("") })

		env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
			tools.RegisterSendRequestTool(s, c)
		})
		setup(env.Mock)

		result := env.CallTool(t, "caido_send_request", map[string]any{
			"raw":    "GET /test HTTP/1.1\r\nHost: example.com\r\n\r\n",
			"host":   "example.com",
			"marker": "not-present-xyz",
		})
		if result.IsError {
			t.Fatalf("unexpected error result: %+v", result)
		}
		output := testutil.UnmarshalToolResult[tools.SendRequestOutput](t, result)

		if output.Reflected == nil || *output.Reflected {
			t.Fatalf("want reflected=false, got %+v", output.Reflected)
		}
	})

	t.Run("no marker leaves reflected unset", func(t *testing.T) {
		replay.ResetDefaultSession("")
		t.Cleanup(func() { replay.ResetDefaultSession("") })

		env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
			tools.RegisterSendRequestTool(s, c)
		})
		setup(env.Mock)

		result := env.CallTool(t, "caido_send_request", map[string]any{
			"raw":  "GET /test HTTP/1.1\r\nHost: example.com\r\n\r\n",
			"host": "example.com",
		})
		if result.IsError {
			t.Fatalf("unexpected error result: %+v", result)
		}
		output := testutil.UnmarshalToolResult[tools.SendRequestOutput](t, result)

		if output.Reflected != nil {
			t.Fatalf("want reflected unset when no marker given, got %+v", *output.Reflected)
		}
	})
}
