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

// TestListRequestsOrder asserts both the GraphQL order input a call sends and
// that the default call keeps sending order: null (no ordering), so existing
// callers see the same query they did before.
func TestListRequestsOrder(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		want     map[string]any
		wantErr  bool
		errParam string
	}{
		{
			name:  "default sends no order",
			input: map[string]any{},
		},
		{
			name:  "desc alone sorts by id",
			input: map[string]any{"order": "desc"},
			want:  map[string]any{"by": "ID", "ordering": "DESC"},
		},
		{
			name:  "orderBy with asc",
			input: map[string]any{"orderBy": "createdAt", "order": "asc"},
			want:  map[string]any{"by": "CREATED_AT", "ordering": "ASC"},
		},
		{
			name:  "orderBy alone sorts descending",
			input: map[string]any{"orderBy": "responseLength"},
			want:  map[string]any{"by": "RESP_LENGTH", "ordering": "DESC"},
		},
		{
			name:  "mapped field name",
			input: map[string]any{"orderBy": "roundtripTime", "order": "desc"},
			want:  map[string]any{"by": "RESP_ROUNDTRIP_TIME", "ordering": "DESC"},
		},
		{
			name:     "rejects unknown orderBy",
			input:    map[string]any{"orderBy": "size", "order": "desc"},
			wantErr:  true,
			errParam: "orderBy",
		},
		{
			name:     "rejects unknown order",
			input:    map[string]any{"order": "descending"},
			wantErr:  true,
			errParam: "order",
		},
		{
			name:     "rejects uppercase order",
			input:    map[string]any{"order": "DESC"},
			wantErr:  true,
			errParam: "order",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(server *mcp.Server, client *caido.Client) {
				tools.RegisterListRequestsTool(server, client)
			})
			env.Mock.ValidateAgainstSchema()
			env.Mock.On("ListRequests", testutil.ListRequestsResponse("req1"))

			result := env.CallTool(t, "caido_list_requests", tt.input)

			if tt.wantErr {
				if !result.IsError {
					t.Fatal("expected error, got success")
				}
				if tt.errParam != "" && !strings.Contains(errText(result), tt.errParam) {
					t.Errorf(
						"error %q does not name %q", errText(result), tt.errParam,
					)
				}
				if vars := env.Mock.LastVariables("ListRequests"); vars != nil {
					t.Errorf("invalid input still reached the API: %v", vars)
				}
				return
			}

			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}

			vars := env.Mock.LastVariables("ListRequests")
			if vars == nil {
				t.Fatal("ListRequests was never called")
			}

			got, _ := vars["order"].(map[string]any)
			if tt.want == nil {
				if vars["order"] != nil {
					t.Errorf("order sent by default: %v", vars["order"])
				}
				return
			}
			if got == nil {
				t.Fatalf("no order sent, want %v", tt.want)
			}
			for k, want := range tt.want {
				if got[k] != want {
					t.Errorf("order.%s = %v, want %v", k, got[k], want)
				}
			}
		})
	}
}

// TestListRequestsTotalCount asserts totalCount reports every match for the
// filter, not just the rows on this page.
func TestListRequestsTotalCount(t *testing.T) {
	tests := []struct {
		name    string
		ids     []string
		count   int
		wantLen int
	}{
		{
			name:    "page smaller than total",
			ids:     []string{"req1", "req2"},
			count:   554,
			wantLen: 2,
		},
		{
			name:    "total matches page",
			ids:     []string{"req1"},
			count:   1,
			wantLen: 1,
		},
		{
			name:    "no matches",
			ids:     nil,
			count:   0,
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(server *mcp.Server, client *caido.Client) {
				tools.RegisterListRequestsTool(server, client)
			})
			env.Mock.ValidateAgainstSchema()

			mockData := testutil.ListRequestsResponse(tt.ids...)
			conn, _ := mockData["requests"].(map[string]any)
			conn["count"] = map[string]any{"value": tt.count}
			env.Mock.On("ListRequests", mockData)

			result := env.CallTool(t, "caido_list_requests", map[string]any{
				"limit": 2,
			})
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}

			output := testutil.UnmarshalToolResult[tools.ListRequestsOutput](t, result)

			if output.TotalCount != tt.count {
				t.Errorf("got TotalCount=%d, want %d", output.TotalCount, tt.count)
			}
			if len(output.Requests) != tt.wantLen {
				t.Errorf("got %d requests, want %d", len(output.Requests), tt.wantLen)
			}
		})
	}
}

// TestListRequestsOrderedPagination checks that a second page keeps the order
// and the cursor from the first, which is what makes desc paging contiguous.
func TestListRequestsOrderedPagination(t *testing.T) {
	env := testutil.NewMCPTestEnv(t, func(server *mcp.Server, client *caido.Client) {
		tools.RegisterListRequestsTool(server, client)
	})
	env.Mock.ValidateAgainstSchema()

	env.Mock.On("ListRequests", map[string]any{
		"requests": map[string]any{
			"edges": []map[string]any{
				{
					"node": map[string]any{
						"id":     "req20",
						"method": "GET",
						"host":   "example.com",
						"port":   443,
						"path":   "/api/20",
						"query":  "",
						"isTls":  true,
						"response": map[string]any{
							"statusCode": 200,
						},
					},
				},
			},
			"pageInfo": map[string]any{
				"hasNextPage": true,
				"endCursor":   "cursor-page-1",
			},
			"count": map[string]any{"value": 20},
		},
	})

	first := env.CallTool(t, "caido_list_requests", map[string]any{
		"limit": 1,
		"order": "desc",
	})
	if first.IsError {
		t.Fatalf("unexpected error: %v", first.Content)
	}
	page1 := testutil.UnmarshalToolResult[tools.ListRequestsOutput](t, first)
	if page1.NextCursor != "cursor-page-1" {
		t.Fatalf("got NextCursor=%q, want cursor-page-1", page1.NextCursor)
	}

	env.Mock.On("ListRequests", testutil.ListRequestsResponse("req19"))
	second := env.CallTool(t, "caido_list_requests", map[string]any{
		"limit": 1,
		"order": "desc",
		"after": page1.NextCursor,
	})
	if second.IsError {
		t.Fatalf("unexpected error: %v", second.Content)
	}

	vars := env.Mock.LastVariables("ListRequests")
	if vars["after"] != page1.NextCursor {
		t.Errorf("page 2 sent after=%v, want %q", vars["after"], page1.NextCursor)
	}
	order, _ := vars["order"].(map[string]any)
	if order["by"] != "ID" || order["ordering"] != "DESC" {
		t.Errorf("page 2 sent order=%v, want ID/DESC", vars["order"])
	}
}

// errText flattens a tool error result into one string for substring checks.
func errText(result *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
			b.WriteString(" ")
		}
	}
	return b.String()
}
