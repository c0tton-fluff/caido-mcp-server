package tools_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/testutil"
	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/tools"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const snippetKey = "AKIAQ3EXAMPLEKEY7777"

// snippetRawResponse builds a base64 raw HTTP response with the given body.
func snippetRawResponse(contentType, body string) string {
	raw := fmt.Sprintf(
		"HTTP/1.1 200 OK\r\nContent-Type: %s\r\nContent-Length: %d\r\n\r\n%s",
		contentType, len(body), body,
	)
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

// snippetGetRequestResponse builds a GetRequest mock payload for one row.
func snippetGetRequestResponse(id, requestBody, responseBody string) map[string]any {
	rawReq := fmt.Sprintf(
		"POST /upload HTTP/1.1\r\nHost: static.localhost\r\n\r\n%s",
		requestBody,
	)
	return map[string]any{
		"request": map[string]any{
			"id":        id,
			"method":    "GET",
			"host":      "static.localhost",
			"port":      18080,
			"path":      "/assets/vendor.3c9e.js",
			"query":     "",
			"isTls":     false,
			"createdAt": int64(1714900000000),
			"raw":       base64.StdEncoding.EncodeToString([]byte(rawReq)),
			"response": map[string]any{
				"statusCode":    200,
				"roundtripTime": 12,
				"length":        len(responseBody),
				"raw":           snippetRawResponse("application/javascript", responseBody),
			},
		},
	}
}

func newListRequestsEnv(t *testing.T, data map[string]any) *testutil.MCPTestEnv {
	t.Helper()
	env := testutil.NewMCPTestEnv(t, func(server *mcp.Server, client *caido.Client) {
		tools.RegisterListRequestsTool(server, client)
	})
	env.Mock.ValidateAgainstSchema()
	env.Mock.On("ListRequests", data)
	return env
}

// TestListRequestsSnippetOffsetsAndContext asserts the reported byte offset,
// part and excerpt around a mid-body key.
func TestListRequestsSnippetOffsetsAndContext(t *testing.T) {
	body := strings.Repeat("x", 100) + snippetKey + strings.Repeat("y", 100)
	offset := strings.Index(body, snippetKey)

	env := newListRequestsEnv(t, testutil.ListRequestsResponse("req-js"))
	env.Mock.On("GetRequest", snippetGetRequestResponse("req-js", "", body))

	result := env.CallTool(t, "caido_list_requests", map[string]any{
		"limit":   5,
		"snippet": map[string]any{"regex": snippetKey},
	})
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	out := testutil.UnmarshalToolResult[tools.ListRequestsOutput](t, result)
	if len(out.Requests) != 1 {
		t.Fatalf("got %d rows, want 1", len(out.Requests))
	}
	matches := out.Requests[0].Matches
	if len(matches) != 1 {
		t.Fatalf("got %d matches, want 1", len(matches))
	}
	m := matches[0]
	if m.Part != "responseBody" {
		t.Errorf("part = %q, want responseBody", m.Part)
	}
	if m.Offset != offset {
		t.Errorf("offset = %d, want %d", m.Offset, offset)
	}
	wantContext := strings.Repeat("x", 40) + snippetKey + strings.Repeat("y", 40)
	if m.Context != wantContext {
		t.Errorf("context = %q, want %q", m.Context, wantContext)
	}
	if m.ContextOffset != offset-40 {
		t.Errorf("contextOffset = %d, want %d", m.ContextOffset, offset-40)
	}
}

// TestListRequestsSnippetCaps asserts the per-request match cap and the
// context cap are enforced.
func TestListRequestsSnippetCaps(t *testing.T) {
	pad := strings.Repeat("a", 300)
	var b strings.Builder
	for range 12 {
		b.WriteString(pad)
		b.WriteString(snippetKey)
	}
	body := b.String()

	env := newListRequestsEnv(t, testutil.ListRequestsResponse("req-js"))
	env.Mock.On("GetRequest", snippetGetRequestResponse("req-js", "", body))

	result := env.CallTool(t, "caido_list_requests", map[string]any{
		"limit": 5,
		"snippet": map[string]any{
			"regex":         snippetKey,
			"maxPerRequest": 100,
			"context":       5000,
		},
	})
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	out := testutil.UnmarshalToolResult[tools.ListRequestsOutput](t, result)
	row := out.Requests[0]
	if len(row.Matches) != 10 {
		t.Fatalf("got %d matches, want the cap of 10", len(row.Matches))
	}
	if !row.MatchesTruncated {
		t.Error("expected matchesTruncated for 12 matches over a cap of 10")
	}

	env2 := newListRequestsEnv(t, testutil.ListRequestsResponse("req-js"))
	env2.Mock.On("GetRequest", snippetGetRequestResponse("req-js", "", body))
	result2 := env2.CallTool(t, "caido_list_requests", map[string]any{
		"limit": 5,
		"snippet": map[string]any{
			"regex":         snippetKey,
			"maxPerRequest": 1,
			"context":       5000,
		},
	})
	out2 := testutil.UnmarshalToolResult[tools.ListRequestsOutput](t, result2)
	if len(out2.Requests[0].Matches) != 1 {
		t.Fatalf("got %d matches, want 1", len(out2.Requests[0].Matches))
	}
	gotContext := len(out2.Requests[0].Matches[0].Context)
	// 200 bytes of context each side is the cap.
	if want := 200 + len(snippetKey) + 200; gotContext != want {
		t.Errorf("context length = %d, want %d", gotContext, want)
	}
}

// TestListRequestsSnippetParts asserts the default part is the response body
// and that request bodies are only scanned when asked for.
func TestListRequestsSnippetParts(t *testing.T) {
	requestBody := "token=" + snippetKey

	env := newListRequestsEnv(t, testutil.ListRequestsResponse("req-js"))
	env.Mock.On(
		"GetRequest",
		snippetGetRequestResponse("req-js", requestBody, "nothing here"),
	)
	result := env.CallTool(t, "caido_list_requests", map[string]any{
		"limit":   5,
		"snippet": map[string]any{"regex": snippetKey},
	})
	out := testutil.UnmarshalToolResult[tools.ListRequestsOutput](t, result)
	if len(out.Requests[0].Matches) != 0 {
		t.Fatalf(
			"default scan found %d matches outside responseBody",
			len(out.Requests[0].Matches),
		)
	}

	env2 := newListRequestsEnv(t, testutil.ListRequestsResponse("req-js"))
	env2.Mock.On(
		"GetRequest",
		snippetGetRequestResponse("req-js", requestBody, "nothing here"),
	)
	result2 := env2.CallTool(t, "caido_list_requests", map[string]any{
		"limit": 5,
		"snippet": map[string]any{
			"regex": snippetKey,
			"parts": []string{"requestBody"},
		},
	})
	out2 := testutil.UnmarshalToolResult[tools.ListRequestsOutput](t, result2)
	matches := out2.Requests[0].Matches
	if len(matches) != 1 {
		t.Fatalf("got %d matches, want 1", len(matches))
	}
	if matches[0].Part != "requestBody" {
		t.Errorf("part = %q, want requestBody", matches[0].Part)
	}
}

// TestListRequestsSnippetScanTruncated asserts a body larger than the scan cap
// is reported rather than read in full.
func TestListRequestsSnippetScanTruncated(t *testing.T) {
	body := strings.Repeat("a", 1<<20) + snippetKey

	env := newListRequestsEnv(t, testutil.ListRequestsResponse("req-js"))
	env.Mock.On("GetRequest", snippetGetRequestResponse("req-js", "", body))
	result := env.CallTool(t, "caido_list_requests", map[string]any{
		"limit":   5,
		"snippet": map[string]any{"regex": snippetKey},
	})
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	out := testutil.UnmarshalToolResult[tools.ListRequestsOutput](t, result)
	row := out.Requests[0]
	if len(row.Matches) != 0 {
		t.Errorf("found %d matches past the scan cap", len(row.Matches))
	}
	if !row.ScanTruncated {
		t.Error("expected scanTruncated for a body over 1 MiB")
	}
}

// TestListRequestsSnippetContextRuneBoundary asserts a context window that
// starts or ends inside a multi-byte rune is moved to a rune boundary.
func TestListRequestsSnippetContextRuneBoundary(t *testing.T) {
	body := strings.Repeat("é", 10) + snippetKey + strings.Repeat("é", 10)

	env := newListRequestsEnv(t, testutil.ListRequestsResponse("req-js"))
	env.Mock.On("GetRequest", snippetGetRequestResponse("req-js", "", body))
	result := env.CallTool(t, "caido_list_requests", map[string]any{
		"limit": 5,
		"snippet": map[string]any{
			"regex":   snippetKey,
			"context": 3,
		},
	})
	out := testutil.UnmarshalToolResult[tools.ListRequestsOutput](t, result)
	matches := out.Requests[0].Matches
	if len(matches) != 1 {
		t.Fatalf("got %d matches, want 1", len(matches))
	}
	if !utf8.ValidString(matches[0].Context) {
		t.Errorf("context is not valid UTF-8: %q", matches[0].Context)
	}
	if !strings.Contains(matches[0].Context, snippetKey) {
		t.Errorf("context lost the match: %q", matches[0].Context)
	}
}

// TestListRequestsSnippetValidation asserts invalid snippet input fails before
// the list query is sent.
func TestListRequestsSnippetValidation(t *testing.T) {
	tests := []struct {
		name    string
		snippet map[string]any
		want    string
	}{
		{
			name:    "empty matching regex",
			snippet: map[string]any{"regex": "a*"},
			want:    "empty string",
		},
		{
			name:    "invalid regex",
			snippet: map[string]any{"regex": "(["},
			want:    "RE2",
		},
		{
			name:    "unknown part",
			snippet: map[string]any{"regex": "x", "parts": []string{"raw"}},
			want:    "parts",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newListRequestsEnv(t, testutil.ListRequestsResponse("req-1"))
			result := env.CallTool(t, "caido_list_requests", map[string]any{
				"limit":   5,
				"snippet": tt.snippet,
			})
			if !result.IsError {
				t.Fatal("expected error")
			}
			text, _ := result.Content[0].(*mcp.TextContent)
			if text == nil || !strings.Contains(text.Text, tt.want) {
				t.Fatalf("error does not mention %q: %v", tt.want, text)
			}
			if vars := env.Mock.LastVariables("ListRequests"); vars != nil {
				t.Errorf("invalid snippet still reached the API: %v", vars)
			}
		})
	}
}

// TestListRequestsFields asserts the extra fields are copied from the row the
// query already returned, including zero and empty values.
func TestListRequestsFields(t *testing.T) {
	data := map[string]any{
		"requests": map[string]any{
			"edges": []map[string]any{
				{
					"node": map[string]any{
						"id":         "req-1",
						"method":     "GET",
						"host":       "api.localhost",
						"port":       18080,
						"path":       "/api/v1/users",
						"query":      "",
						"isTls":      false,
						"createdAt":  int64(1714900000000),
						"length":     123,
						"source":     "REPLAY",
						"alteration": "NONE",
						"response": map[string]any{
							"statusCode":    200,
							"roundtripTime": 0,
							"length":        0,
						},
					},
				},
			},
			"pageInfo": map[string]any{"hasNextPage": false, "endCursor": nil},
			"count":    map[string]any{"value": 1},
		},
	}

	env := newListRequestsEnv(t, data)
	result := env.CallTool(t, "caido_list_requests", map[string]any{
		"limit": 5,
		"fields": []string{
			"createdAt", "host", "path", "query", "source",
			"responseLength", "roundtripTime",
		},
	})
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	out := testutil.UnmarshalToolResult[tools.ListRequestsOutput](t, result)
	if len(out.Requests) != 1 {
		t.Fatalf("got %d rows, want 1", len(out.Requests))
	}
	row := out.Requests[0]
	wantCreated := time.UnixMilli(1714900000000).Format(time.RFC3339)
	if row.CreatedAt == nil || *row.CreatedAt != wantCreated {
		t.Errorf("createdAt = %v, want %q", row.CreatedAt, wantCreated)
	}
	if row.Host == nil || *row.Host != "api.localhost" {
		t.Errorf("host = %v", out.Requests[0].Host)
	}
	if row.Path == nil || *row.Path != "/api/v1/users" {
		t.Errorf("path = %v", row.Path)
	}
	if row.Query == nil || *row.Query != "" {
		t.Errorf("query = %v, want an explicit empty string", row.Query)
	}
	if row.Source == nil || *row.Source != "REPLAY" {
		t.Errorf("source = %v", row.Source)
	}
	if row.ResponseLength == nil || *row.ResponseLength != 0 {
		t.Errorf("responseLength = %v, want 0", row.ResponseLength)
	}
	if row.RoundtripTime == nil || *row.RoundtripTime != 0 {
		t.Errorf("roundtripTime = %v, want 0", row.RoundtripTime)
	}
}

// TestListRequestsFieldsDefaultShape asserts a call without fields marshals the
// same four keys as before.
func TestListRequestsFieldsDefaultShape(t *testing.T) {
	env := newListRequestsEnv(t, testutil.ListRequestsResponse("req-1"))
	result := env.CallTool(t, "caido_list_requests", map[string]any{"limit": 5})
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
	out := testutil.UnmarshalToolResult[tools.ListRequestsOutput](t, result)
	raw, err := json.Marshal(out.Requests[0])
	if err != nil {
		t.Fatal(err)
	}
	want := `{"id":"req-1","method":"GET","url":"https://example.com/api/req-1","statusCode":200}`
	if string(raw) != want {
		t.Fatalf("default row = %s, want %s", raw, want)
	}
}

// TestListRequestsFieldsValidation asserts an unknown field name fails before
// the API is called.
func TestListRequestsFieldsValidation(t *testing.T) {
	env := newListRequestsEnv(t, testutil.ListRequestsResponse("req-1"))
	result := env.CallTool(t, "caido_list_requests", map[string]any{
		"limit":  5,
		"fields": []string{"cookie"},
	})
	if !result.IsError {
		t.Fatal("expected error")
	}
	text, _ := result.Content[0].(*mcp.TextContent)
	if text == nil || !strings.Contains(text.Text, "fields") {
		t.Fatalf("error does not name fields: %v", result.Content)
	}
	if vars := env.Mock.LastVariables("ListRequests"); vars != nil {
		t.Errorf("invalid fields still reached the API: %v", vars)
	}
}

// TestListRequestsSnippetSchema asserts the snippet and fields options
// advertise their closed value sets.
func TestListRequestsSnippetSchema(t *testing.T) {
	schema := advertisedSchema(t, "caido_list_requests")

	props, _ := schema["properties"].(map[string]any)
	if _, ok := props["snippet"]; !ok {
		t.Fatal("snippet is not in the tool schema")
	}
	fieldsProp, _ := props["fields"].(map[string]any)
	fieldsItems, _ := fieldsProp["items"].(map[string]any)
	gotFields, _ := fieldsItems["enum"].([]any)
	if len(gotFields) != 7 {
		t.Fatalf("fields items enum = %v", fieldsItems["enum"])
	}

	snippetProp, _ := props["snippet"].(map[string]any)
	snippetProps, _ := snippetProp["properties"].(map[string]any)
	partsProp, _ := snippetProps["parts"].(map[string]any)
	partsItems, _ := partsProp["items"].(map[string]any)
	parts, _ := partsItems["enum"].([]any)
	wantParts := []string{
		"responseBody", "requestBody", "responseHeaders", "requestHeaders",
	}
	if fmt.Sprint(parts) != fmt.Sprint(wantParts) {
		t.Errorf("snippet.parts enum = %v, want %v", parts, wantParts)
	}
	regexProp, _ := snippetProps["regex"].(map[string]any)
	if ml, _ := regexProp["maxLength"].(float64); ml != 1000 {
		t.Errorf("snippet.regex maxLength = %v, want 1000", ml)
	}
}

// listResponseWithSource builds a one-row list response with a request source.
func listResponseWithSource(source string) map[string]any {
	node := map[string]any{
		"id":     "req-1",
		"method": "GET",
		"host":   "example.com",
		"port":   443,
		"path":   "/api/req-1",
		"query":  "",
		"isTls":  true,
		"response": map[string]any{
			"statusCode": 200,
		},
	}
	if source != "" {
		node["source"] = source
	}
	return map[string]any{
		"requests": map[string]any{
			"edges":    []map[string]any{{"node": node}},
			"pageInfo": map[string]any{"hasNextPage": false, "endCursor": nil},
			"count":    map[string]any{"value": 1},
		},
	}
}

// TestListRequestsSourceOnNonProxyRows asserts rows that did not come through
// the proxy carry their source without being asked, while captured proxy rows
// keep their previous four keys.
func TestListRequestsSourceOnNonProxyRows(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{"replay", "REPLAY", "REPLAY"},
		{"automate", "AUTOMATE", "AUTOMATE"},
		{"workflow", "WORKFLOW", "WORKFLOW"},
		{"plugin", "PLUGIN", "PLUGIN"},
		{"import", "IMPORT", "IMPORT"},
		{"sample", "SAMPLE", "SAMPLE"},
		{"intercept stays out", "INTERCEPT", ""},
		{"empty stays out", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newListRequestsEnv(t, listResponseWithSource(tt.source))
			result := env.CallTool(t, "caido_list_requests", map[string]any{"limit": 5})
			if result.IsError {
				t.Fatalf("unexpected error: %v", result.Content)
			}
			out := testutil.UnmarshalToolResult[tools.ListRequestsOutput](t, result)
			row := out.Requests[0]
			if tt.want == "" {
				if row.Source != nil {
					t.Fatalf("source = %q, want it omitted", *row.Source)
				}
				return
			}
			if row.Source == nil || *row.Source != tt.want {
				t.Fatalf("source = %v, want %q", row.Source, tt.want)
			}
		})
	}
}

// TestListRequestsInterceptRowShape asserts a captured proxy row marshals the
// same four keys as before, even when the API reports its source.
func TestListRequestsInterceptRowShape(t *testing.T) {
	env := newListRequestsEnv(t, listResponseWithSource("INTERCEPT"))
	result := env.CallTool(t, "caido_list_requests", map[string]any{"limit": 5})
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
	out := testutil.UnmarshalToolResult[tools.ListRequestsOutput](t, result)
	raw, err := json.Marshal(out.Requests[0])
	if err != nil {
		t.Fatal(err)
	}
	want := `{"id":"req-1","method":"GET","url":"https://example.com/api/req-1","statusCode":200}`
	if string(raw) != want {
		t.Fatalf("intercept row = %s, want %s", raw, want)
	}
}

// TestListRequestsFieldsSourceOnIntercept asserts fields:["source"] still
// emits the source for a captured proxy row.
func TestListRequestsFieldsSourceOnIntercept(t *testing.T) {
	env := newListRequestsEnv(t, listResponseWithSource("INTERCEPT"))
	result := env.CallTool(t, "caido_list_requests", map[string]any{
		"limit":  5,
		"fields": []string{"source"},
	})
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
	out := testutil.UnmarshalToolResult[tools.ListRequestsOutput](t, result)
	row := out.Requests[0]
	if row.Source == nil || *row.Source != "INTERCEPT" {
		t.Fatalf("source = %v, want INTERCEPT", row.Source)
	}
	if row.Host != nil || row.Path != nil || row.CreatedAt != nil {
		t.Errorf("unrequested fields were set: %+v", row)
	}
}
