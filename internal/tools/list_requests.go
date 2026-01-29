package tools

import (
	"context"
	"fmt"

	"github.com/c0tton-fluff/caido-mcp-server/internal/caido"
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
func listRequestsHandler(client *caido.Client) func(context.Context, *mcp.CallToolRequest, ListRequestsInput) (*mcp.CallToolResult, ListRequestsOutput, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input ListRequestsInput) (*mcp.CallToolResult, ListRequestsOutput, error) {
		limit := input.Limit
		if limit <= 0 {
			limit = 10 // Small default to save context
		}
		if limit > 100 {
			limit = 100
		}

		opts := caido.ListRequestsOptions{
			First:  limit,
			After:  input.After,
			Filter: input.HTTPQL,
		}

		result, err := client.ListRequests(ctx, opts)
		if err != nil {
			return nil, ListRequestsOutput{}, fmt.Errorf("failed to list requests: %w", err)
		}

		output := ListRequestsOutput{
			Requests:   make([]RequestSummary, 0, len(result.Requests.Edges)),
			HasMore:    result.Requests.PageInfo.HasNextPage,
			NextCursor: result.Requests.PageInfo.EndCursor,
		}

		for _, edge := range result.Requests.Edges {
			r := edge.Node
			url := buildURL(r.IsTLS, r.Host, r.Port, r.Path, r.Query)

			summary := RequestSummary{
				ID:     r.ID,
				Method: r.Method,
				URL:    url,
			}

			if r.Response != nil {
				summary.StatusCode = r.Response.StatusCode
			}

			output.Requests = append(output.Requests, summary)
		}

		// Return nil for result, the SDK will create one from the output
		return nil, output, nil
	}
}

// buildURL constructs the full URL from request parts
func buildURL(isTLS bool, host string, port int, path string, query string) string {
	scheme := "http"
	if isTLS {
		scheme = "https"
	}

	url := fmt.Sprintf("%s://%s", scheme, host)

	// Add port if non-standard
	if (isTLS && port != 443) || (!isTLS && port != 80) {
		url = fmt.Sprintf("%s:%d", url, port)
	}

	url += path

	if query != "" {
		url += "?" + query
	}

	return url
}

// RegisterListRequestsTool registers the tool with the MCP server
func RegisterListRequestsTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_list_requests",
		Description: `List HTTP requests. Filter with httpql (e.g. req.host.eq:"example.com"). Returns id/method/url/status.`,
	}, listRequestsHandler(client))
}
