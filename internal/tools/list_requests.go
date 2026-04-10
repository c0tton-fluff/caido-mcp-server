package tools

import (
	"context"
	"fmt"

	"github.com/c0tton-fluff/caido-mcp-server/internal/httputil"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ListRequestsInput is the input for the list_requests tool
type ListRequestsInput struct {
	HTTPQL string `json:"httpql,omitempty" jsonschema:"HTTPQL filter query for filtering requests"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum number of requests to return (default 20, max 100)"`
	After  string `json:"after,omitempty" jsonschema:"Cursor for pagination from previous response nextCursor"`
}

// ListRequestsOutput is the output of the list_requests tool
type ListRequestsOutput struct {
	Requests   []RequestSummary `json:"requests"`
	HasMore    bool             `json:"hasMore"`
	NextCursor string           `json:"nextCursor,omitempty"`
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

		limit := input.Limit
		if limit <= 0 {
			limit = 20
		}
		if limit > 100 {
			limit = 100
		}

		opts := &caido.ListRequestsOptions{
			First: &limit,
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

		if conn.PageInfo.HasNextPage {
			output.HasMore = true
			if conn.PageInfo.EndCursor != nil {
				output.NextCursor = *conn.PageInfo.EndCursor
			}
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
		Description: `List HTTP requests. Filter with httpql (e.g. req.host.eq:"example.com"). Returns id/method/url/status.`,
	}, listRequestsHandler(client))
}
