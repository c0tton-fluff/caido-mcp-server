package tools_test

import (
	"testing"

	"github.com/c0tton-fluff/caido-mcp-server/internal/testutil"
	"github.com/c0tton-fluff/caido-mcp-server/internal/tools"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestBatchSend(t *testing.T) {
	tests := []struct {
		name      string
		args      map[string]any
		setup     func(*testutil.MockHandler)
		wantError bool
	}{
		{
			name:      "rejects empty requests array",
			args:      map[string]any{"requests": []map[string]any{}},
			setup:     func(m *testutil.MockHandler) {},
			wantError: true,
		},
		{
			name: "rejects more than 50 requests",
			args: func() map[string]any {
				reqs := make([]map[string]any, 51)
				for i := range reqs {
					reqs[i] = map[string]any{"label": "x", "raw": "GET / HTTP/1.1\r\nHost: x\r\n\r\n"}
				}
				return map[string]any{"requests": reqs}
			}(),
			setup:     func(m *testutil.MockHandler) {},
			wantError: true,
		},
		{
			name: "rejects request with empty raw",
			args: map[string]any{
				"requests": []map[string]any{
					{"label": "test", "raw": ""},
				},
			},
			setup:     func(m *testutil.MockHandler) {},
			wantError: true,
		},
		{
			name: "rejects request with raw over 1MB",
			args: map[string]any{
				"requests": []map[string]any{
					{"label": "big", "raw": string(make([]byte, 1048577))},
				},
			},
			setup:     func(m *testutil.MockHandler) {},
			wantError: true,
		},
		{
			name: "successful batch sends requests",
			args: map[string]any{
				"requests": []map[string]any{
					{"label": "req-a", "raw": "GET /a HTTP/1.1\r\nHost: example.com\r\n\r\n"},
					{"label": "req-b", "raw": "GET /b HTTP/1.1\r\nHost: example.com\r\n\r\n"},
				},
				"concurrency": 2,
			},
			setup: func(m *testutil.MockHandler) {
				m.On("CreateReplaySession", testutil.CreateReplaySessionResponse("batch-s1"))
				m.On("StartReplayTask", testutil.StartReplayTaskResponse())
				m.On("GetReplaySession", testutil.GetReplaySessionResponse("batch-s1", "be-1"))
				m.On("GetReplayEntry", testutil.GetReplayEntryResponse("be-1", "br-1", 200, "ok"))
				m.On("DeleteReplaySessions", map[string]any{
					"deleteReplaySessions": map[string]any{
						"deletedIds": []string{},
					},
				})
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterBatchSendTool(s, c)
			})
			tt.setup(env.Mock)

			result := env.CallTool(t, "caido_batch_send", tt.args)

			if tt.wantError {
				if !result.IsError {
					t.Fatal("expected error result")
				}
				return
			}

			output := testutil.UnmarshalToolResult[tools.BatchSendOutput](t, result)
			if len(output.Results) != 2 {
				t.Fatalf("want 2 results, got %d", len(output.Results))
			}
		})
	}
}

// TestBatchSendFingerprintExpansion covers the Chunk 4 additions:
// includeBody gating (default false), marker/reflected, and the enriched
// fingerprint fields, which for batch_send are computed from the same
// (already bodyLimit-truncated) response replay.RunBatch returns, since
// the raw undecoded response is internal to internal/replay.
func TestBatchSendFingerprintExpansion(t *testing.T) {
	rawResp := rawHTTPResponseWithHeaders(
		200,
		"Set-Cookie: token=zzz; Path=/\r\n",
		"<html><head><title>Batch Page</title></head><body>reflect-me</body></html>",
	)

	setup := func(m *testutil.MockHandler) {
		m.On("CreateReplaySession", testutil.CreateReplaySessionResponse("batch-fp"))
		m.On("StartReplayTask", testutil.StartReplayTaskResponse())
		m.On("GetReplaySession", testutil.GetReplaySessionResponse("batch-fp", "be-fp"))
		m.On("GetReplayEntry", entryResponseWithRaw("be-fp", "br-fp", rawResp, 200))
		m.On("DeleteReplaySessions", map[string]any{
			"deleteReplaySessions": map[string]any{
				"deletedIds": []string{},
			},
		})
	}

	oneRequest := []map[string]any{
		{"label": "a", "raw": "GET /a HTTP/1.1\r\nHost: example.com\r\n\r\n"},
	}

	t.Run("body omitted by default but fingerprint populated", func(t *testing.T) {
		env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
			tools.RegisterBatchSendTool(s, c)
		})
		setup(env.Mock)

		result := env.CallTool(t, "caido_batch_send", map[string]any{
			"requests": oneRequest,
		})
		if result.IsError {
			t.Fatalf("unexpected error result: %+v", result)
		}
		output := testutil.UnmarshalToolResult[tools.BatchSendOutput](t, result)
		if len(output.Results) != 1 {
			t.Fatalf("want 1 result, got %d", len(output.Results))
		}
		r := output.Results[0]
		if r.Response == nil {
			t.Fatalf("expected a response")
		}
		if r.Response.Body != "" {
			t.Fatalf("want empty body when includeBody defaults false, got %q", r.Response.Body)
		}
		fp := r.Response.Fingerprint
		if fp == nil {
			t.Fatalf("expected a fingerprint")
		}
		if fp.StatusCode != 200 {
			t.Fatalf("want fingerprint statusCode 200, got %d", fp.StatusCode)
		}
		if fp.Title != "Batch Page" {
			t.Fatalf("want title %q, got %q", "Batch Page", fp.Title)
		}
		if fp.WordCount == 0 {
			t.Fatalf("want non-zero wordCount")
		}
	})

	t.Run("includeBody true includes body text", func(t *testing.T) {
		env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
			tools.RegisterBatchSendTool(s, c)
		})
		setup(env.Mock)

		result := env.CallTool(t, "caido_batch_send", map[string]any{
			"requests":    oneRequest,
			"includeBody": true,
		})
		if result.IsError {
			t.Fatalf("unexpected error result: %+v", result)
		}
		output := testutil.UnmarshalToolResult[tools.BatchSendOutput](t, result)
		if output.Results[0].Response == nil || output.Results[0].Response.Body == "" {
			t.Fatalf("want body included, got %+v", output.Results[0].Response)
		}
	})

	t.Run("marker sets reflected per result", func(t *testing.T) {
		env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
			tools.RegisterBatchSendTool(s, c)
		})
		setup(env.Mock)

		result := env.CallTool(t, "caido_batch_send", map[string]any{
			"requests": oneRequest,
			"marker":   "reflect-me",
		})
		if result.IsError {
			t.Fatalf("unexpected error result: %+v", result)
		}
		output := testutil.UnmarshalToolResult[tools.BatchSendOutput](t, result)
		r := output.Results[0]
		if r.Reflected == nil || !*r.Reflected {
			t.Fatalf("want reflected=true, got %+v", r.Reflected)
		}
	})

	t.Run("no marker leaves reflected unset", func(t *testing.T) {
		env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
			tools.RegisterBatchSendTool(s, c)
		})
		setup(env.Mock)

		result := env.CallTool(t, "caido_batch_send", map[string]any{
			"requests": oneRequest,
		})
		if result.IsError {
			t.Fatalf("unexpected error result: %+v", result)
		}
		output := testutil.UnmarshalToolResult[tools.BatchSendOutput](t, result)
		if output.Results[0].Reflected != nil {
			t.Fatalf("want reflected unset when no marker given, got %v", *output.Results[0].Reflected)
		}
	})
}
