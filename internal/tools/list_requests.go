package tools

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/httputil"
	caido "github.com/caido-community/sdk-go"
	gen "github.com/caido-community/sdk-go/graphql"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ListRequestsInput is the input for the list_requests tool
type ListRequestsInput struct {
	HTTPQL  string `json:"httpql,omitempty" jsonschema:"HTTPQL filter query for filtering requests"`
	Limit   int    `json:"limit,omitempty" jsonschema:"Maximum number of requests to return (default 20, max 100)"`
	After   string `json:"after,omitempty" jsonschema:"Cursor for pagination from previous response nextCursor"`
	OrderBy string `json:"orderBy,omitempty" jsonschema:"Field to sort by (default id). Omit to leave the server's default order untouched"`
	Order   string `json:"order,omitempty" jsonschema:"Sort direction: asc or desc. Set order without orderBy to sort by id, i.e. desc gives the newest requests first"`
	ScopeID string `json:"scopeId,omitempty" jsonschema:"Only requests inside this scope (ID from caido_list_scopes). Combines with httpql; totalCount then counts in-scope matches"`
	// Snippet runs a RE2 regex over the returned rows and reports matches
	// with context, without returning the bodies.
	Snippet *ListRequestsSnippetInput `json:"snippet,omitempty" jsonschema:"Regex scan over the returned rows: {regex, context, maxPerRequest, parts}"`
	// Fields adds per-row fields that the list query already returns.
	Fields []string `json:"fields,omitempty" jsonschema:"Extra per-row fields: createdAt, host, path, query, source, responseLength, roundtripTime"`
}

// ListRequestsOutput is the output of the list_requests tool
type ListRequestsOutput struct {
	Requests   []RequestSummary `json:"requests"`
	HasMore    bool             `json:"hasMore"`
	NextCursor string           `json:"nextCursor,omitempty"`
	TotalCount int              `json:"totalCount"`
}

// requestOrderFields maps the agent-facing orderBy values onto Caido's
// RequestResponseOrderBy enum. The names follow the request row fields an
// agent already sees (plus createdAt and source, which only the GraphQL
// query carries); Caido's own enum names are SCREAMING_SNAKE, which reads
// badly in a tool parameter.
var requestOrderFields = []struct {
	name string
	by   gen.RequestResponseOrderBy
}{
	{"id", gen.RequestResponseOrderById},
	{"createdAt", gen.RequestResponseOrderByCreatedAt},
	{"host", gen.RequestResponseOrderByHost},
	{"method", gen.RequestResponseOrderByMethod},
	{"path", gen.RequestResponseOrderByPath},
	{"query", gen.RequestResponseOrderByQuery},
	{"fileExtension", gen.RequestResponseOrderByFileExtension},
	{"source", gen.RequestResponseOrderBySource},
	{"statusCode", gen.RequestResponseOrderByRespStatusCode},
	{"roundtripTime", gen.RequestResponseOrderByRespRoundtripTime},
	{"responseLength", gen.RequestResponseOrderByRespLength},
}

// requestOrderNames returns the accepted orderBy values, in table order.
func requestOrderNames() []string {
	names := make([]string, len(requestOrderFields))
	for i, f := range requestOrderFields {
		names[i] = f.name
	}
	return names
}

// buildRequestOrder turns the tool's orderBy/order strings into a GraphQL
// order input. Both empty returns nil, which leaves the `order` variable out
// of the query and keeps the pre-existing (server default) behaviour.
//
// A direction without a field sorts by id, which is the natural "newest
// first". A field without a direction sorts descending: the tool exists to
// triage recent traffic, so newest/largest first is the useful default.
func buildRequestOrder(
	orderBy, order string,
) (*gen.RequestResponseOrderInput, error) {
	if orderBy == "" && order == "" {
		return nil, nil
	}

	by := gen.RequestResponseOrderById
	if orderBy != "" {
		found := false
		for _, f := range requestOrderFields {
			if f.name == orderBy {
				by, found = f.by, true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf(
				"orderBy must be one of [%s], got %q",
				strings.Join(requestOrderNames(), " "), orderBy,
			)
		}
	}

	ordering := gen.OrderingDesc
	switch order {
	case "asc":
		ordering = gen.OrderingAsc
	case "", "desc":
		// desc is the default; "" means orderBy was given alone.
	default:
		return nil, fmt.Errorf(
			"order must be one of [asc desc], got %q", order,
		)
	}

	return &gen.RequestResponseOrderInput{By: by, Ordering: ordering}, nil
}

// RequestSummary is a minimal representation of a request
type RequestSummary struct {
	ID             string  `json:"id"`
	Method         string  `json:"method"`
	URL            string  `json:"url"`
	StatusCode     int     `json:"statusCode,omitempty"`
	CreatedAt      *string `json:"createdAt,omitempty"`
	Host           *string `json:"host,omitempty"`
	Path           *string `json:"path,omitempty"`
	Query          *string `json:"query,omitempty"`
	Source         *string `json:"source,omitempty"`
	ResponseLength *int    `json:"responseLength,omitempty"`
	RoundtripTime  *int    `json:"roundtripTime,omitempty"`
	// Matches holds up to snippet.max regex hits for this row.
	Matches []RequestMatch `json:"matches,omitempty"`
	// MatchesTruncated reports that more matches exist than were returned.
	MatchesTruncated bool `json:"matchesTruncated,omitempty"`
	// ScanTruncated reports that a part exceeded the per-row scan cap.
	ScanTruncated bool `json:"scanTruncated,omitempty"`
	// SnippetError reports why this row could not be scanned.
	SnippetError string `json:"snippetError,omitempty"`
}

// applyRequestFields copies the requested extra fields from a list row and
// marks rows that did not come through the proxy.
func applyRequestFields(
	summary *RequestSummary,
	r gen.ListRequestsRequestsRequestConnectionEdgesRequestEdgeNodeRequest,
	fields map[string]bool,
) {
	if fields != nil {
		if fields["createdAt"] {
			created := time.UnixMilli(r.CreatedAt).Format(time.RFC3339)
			summary.CreatedAt = &created
		}
		if fields["host"] {
			summary.Host = &r.Host
		}
		if fields["path"] {
			summary.Path = &r.Path
		}
		if fields["query"] {
			summary.Query = &r.Query
		}
		if fields["source"] {
			source := string(r.Source)
			summary.Source = &source
		}
		if r.Response != nil {
			if fields["responseLength"] {
				length := r.Response.Length
				summary.ResponseLength = &length
			}
			if fields["roundtripTime"] {
				roundtrip := r.Response.RoundtripTime
				summary.RoundtripTime = &roundtrip
			}
		}
	}

	// Replayed, automated or imported rows say so even when fields was not
	// asked for: a row that is not captured proxy traffic is a different
	// object to an agent looking for the captured request, and the row shape
	// for captured traffic stays unchanged. An empty source is not a real
	// enum value (it appears in fixtures), so it stays out.
	if summary.Source == nil && r.Source != "" &&
		r.Source != gen.SourceIntercept {
		source := string(r.Source)
		summary.Source = &source
	}
}

// requestFieldNames is the accepted set for the fields option. Each name
// maps to a value the list query already returns, so no extra request is
// needed.
var requestFieldNames = []string{
	"createdAt", "host", "path", "query", "source", "responseLength",
	"roundtripTime",
}

// parseRequestFields validates the fields option.
func parseRequestFields(fields []string) (map[string]bool, error) {
	if len(fields) == 0 {
		return nil, nil
	}
	selected := make(map[string]bool, len(fields))
	for _, f := range fields {
		if !slices.Contains(requestFieldNames, f) {
			return nil, fmt.Errorf(
				"fields must be one of [%s], got %q",
				strings.Join(requestFieldNames, " "), f,
			)
		}
		selected[f] = true
	}
	return selected, nil
}

// listRequestsHandler creates the handler function for the list_requests tool
func listRequestsHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, ListRequestsInput) (*mcp.CallToolResult, ListRequestsOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input ListRequestsInput,
	) (*mcp.CallToolResult, ListRequestsOutput, error) {
		if len(input.HTTPQL) > 10000 {
			return nil, ListRequestsOutput{}, fmt.Errorf(
				"httpql exceeds max length of 10000",
			)
		}

		limit := clampLimit(input.Limit, 20, 100)

		order, err := buildRequestOrder(input.OrderBy, input.Order)
		if err != nil {
			return nil, ListRequestsOutput{}, err
		}

		fields, err := parseRequestFields(input.Fields)
		if err != nil {
			return nil, ListRequestsOutput{}, err
		}
		snippet, err := newSnippetSpec(input.Snippet)
		if err != nil {
			return nil, ListRequestsOutput{}, err
		}

		opts := &caido.ListRequestsOptions{
			First: &limit,
			Order: order,
		}
		if input.HTTPQL != "" {
			opts.Filter = &input.HTTPQL
		}
		if input.ScopeID != "" {
			if err := requireScope(ctx, client, input.ScopeID); err != nil {
				return nil, ListRequestsOutput{}, err
			}
			opts.ScopeID = &input.ScopeID
		}
		if input.After != "" {
			opts.After = &input.After
		}

		resp, err := client.Requests.List(ctx, opts)
		if err != nil {
			return nil, ListRequestsOutput{}, fmt.Errorf(
				"failed to list requests: %w", err,
			)
		}

		conn := resp.Requests
		output := ListRequestsOutput{
			Requests: make([]RequestSummary, 0, len(conn.Edges)),
		}

		output.TotalCount = conn.Count.Value

		for _, edge := range conn.Edges {
			r := edge.Node
			summary := RequestSummary{
				ID:     r.Id,
				Method: r.Method,
				URL: httputil.BuildURL(
					r.IsTls, r.Host, r.Port, r.Path, r.Query,
				),
			}
			if r.Response != nil {
				summary.StatusCode = r.Response.StatusCode
			}
			applyRequestFields(&summary, r, fields)
			output.Requests = append(output.Requests, summary)
		}

		output.HasMore, output.NextCursor = pageCursor(
			conn.PageInfo.HasNextPage, conn.PageInfo.EndCursor,
		)

		if snippet != nil {
			snippet.annotate(ctx, client, output.Requests)
		}

		return nil, output, nil
	}
}

// RegisterListRequestsTool registers the tool with the MCP server
func RegisterListRequestsTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_list_requests",
		Title:       "List Requests",
		Description: `List HTTP requests. Filter with httpql (e.g. req.host.eq:"example.com"). On Caido 0.58+ match headers and bodies directly instead of scanning raw: req.header["Authorization"].cont:"Bearer", resp.body.cont:"stack trace". Returns id/method/url/status, and totalCount = how many requests match httpql. Newest first in one call: limit:5 with order:"desc" (sorts by id); orderBy picks the field (createdAt, statusCode, roundtripTime, responseLength, host, path, ...). Counting matches is limit:1 plus totalCount; no paging needed. scopeId restricts the listing and the count to one scope (caido_list_scopes). See a value without downloading bodies: snippet:{regex:"AKIA[0-9A-Z]{16}"} returns each hit with its part, byte offset and context instead of the bodies. fields adds createdAt/host/path/query/source/responseLength/roundtripTime to each row, and rows that did not come from the proxy carry a source (REPLAY, AUTOMATE, WORKFLOW, PLUGIN, IMPORT) without asking. Paginate with after from nextCursor, which keeps the same order.`,
		InputSchema: schemaFor[ListRequestsInput](func(s *jsonschema.Schema) {
			if p := prop(s, "httpql"); p != nil {
				p.MaxLength = intPtr(10000)
			}
			if p := prop(s, "orderBy"); p != nil {
				p.Enum = anyStrings(requestOrderNames())
			}
			if p := prop(s, "order"); p != nil {
				p.Enum = []any{"asc", "desc"}
			}
			if p := prop(s, "fields"); p != nil {
				if p.Items != nil {
					p.Items.Enum = anyStrings(requestFieldNames)
				}
			}
			if snippet := prop(s, "snippet"); snippet != nil {
				if p := prop(snippet, "regex"); p != nil {
					p.MaxLength = intPtr(1000)
				}
				if parts := prop(snippet, "parts"); parts != nil {
					if parts.Items != nil {
						parts.Items.Enum = anyStrings(snippetParts)
					}
				}
			}
		}),
		Annotations: readOnlyAnn(),
	}, listRequestsHandler(client))
}
