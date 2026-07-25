package tools_test

import (
	"strings"
	"testing"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/testutil"
	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/tools"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestListRequests(t *testing.T) {
	tests := []struct {
		name        string
		input       map[string]any
		mockData    map[string]any
		wantErr     bool
		wantReqCnt  int
		wantHasMore bool
		wantCursor  string
	}{
		{
			name:  "returns requests with default limit",
			input: map[string]any{},
			mockData: testutil.ListRequestsResponse(
				"req1", "req2", "req3",
			),
			wantReqCnt:  3,
			wantHasMore: false,
		},
		{
			name: "passes httpql filter",
			input: map[string]any{
				"httpql": `req.host.eq:"example.com"`,
			},
			mockData: testutil.ListRequestsResponse(
				"req1", "req2",
			),
			wantReqCnt:  2,
			wantHasMore: false,
		},
		{
			name: "rejects httpql over 10000 chars",
			input: map[string]any{
				"httpql": strings.Repeat("a", 10001),
			},
			wantErr: true,
		},
		{
			name:        "handles empty response",
			input:       map[string]any{},
			mockData:    testutil.ListRequestsResponse(),
			wantReqCnt:  0,
			wantHasMore: false,
		},
		{
			name: "pagination with hasMore and nextCursor",
			input: map[string]any{
				"limit": 2,
			},
			mockData: map[string]any{
				"requests": map[string]any{
					"edges": []map[string]any{
						{
							"node": map[string]any{
								"id":     "req1",
								"method": "GET",
								"host":   "example.com",
								"port":   443,
								"path":   "/api/1",
								"query":  "",
								"isTls":  true,
								"response": map[string]any{
									"statusCode": 200,
								},
							},
						},
						{
							"node": map[string]any{
								"id":     "req2",
								"method": "POST",
								"host":   "api.example.com",
								"port":   443,
								"path":   "/v1/data",
								"query":  "foo=bar",
								"isTls":  true,
								"response": map[string]any{
									"statusCode": 201,
								},
							},
						},
					},
					"pageInfo": map[string]any{
						"hasNextPage": true,
						"endCursor":   "cursor123",
					},
				},
			},
			wantReqCnt:  2,
			wantHasMore: true,
			wantCursor:  "cursor123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(server *mcp.Server, client *caido.Client) {
				tools.RegisterListRequestsTool(server, client)
			})

			if !tt.wantErr {
				env.Mock.On("ListRequests", tt.mockData)
			}

			result := env.CallTool(t, "caido_list_requests", tt.input)

			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error, got success")
				}
				return
			}

			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}

			output := testutil.UnmarshalToolResult[tools.ListRequestsOutput](t, result)

			if len(output.Requests) != tt.wantReqCnt {
				t.Errorf("got %d requests, want %d", len(output.Requests), tt.wantReqCnt)
			}

			if output.HasMore != tt.wantHasMore {
				t.Errorf("got HasMore=%v, want %v", output.HasMore, tt.wantHasMore)
			}

			if tt.wantCursor != "" && output.NextCursor != tt.wantCursor {
				t.Errorf("got NextCursor=%q, want %q", output.NextCursor, tt.wantCursor)
			}

			if tt.wantReqCnt > 0 {
				req := output.Requests[0]
				if req.ID == "" {
					t.Error("request ID is empty")
				}
				if req.Method == "" {
					t.Error("request Method is empty")
				}
				if req.URL == "" {
					t.Error("request URL is empty")
				}
			}
		})
	}
}
