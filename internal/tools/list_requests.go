package tools

import (
	"context"
	"fmt"
	"strings"

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
	ID         string `json:"id"`
	Method     string `json:"method"`
	URL        string `json:"url"`
	StatusCode int    `json:"statusCode,omitempty"`
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

		opts := &caido.ListRequestsOptions{
			First: &limit,
			Order: order,
		}
		if input.HTTPQL != "" {
			opts.Filter = &input.HTTPQL
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
			output.Requests = append(output.Requests, summary)
		}

		output.HasMore, output.NextCursor = pageCursor(
			conn.PageInfo.HasNextPage, conn.PageInfo.EndCursor,
		)

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
		Description: `List HTTP requests. Filter with httpql (e.g. req.host.eq:"example.com"). On Caido 0.58+ match headers and bodies directly instead of scanning raw: req.header["Authorization"].cont:"Bearer", resp.body.cont:"stack trace". Returns id/method/url/status, and totalCount = how many requests match httpql. Newest first in one call: limit:5 with order:"desc" (sorts by id); orderBy picks the field (createdAt, statusCode, roundtripTime, responseLength, host, path, ...). Counting matches is limit:1 plus totalCount; no paging needed. Paginate with after from nextCursor, which keeps the same order.`,
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
		}),
		Annotations: readOnlyAnn(),
	}, listRequestsHandler(client))
}
