package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/c0tton-fluff/caido-mcp-server/internal/httputil"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ListInterceptEntriesInput is the input for the tool
type ListInterceptEntriesInput struct {
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum entries to return (default 20, max 100)"`
	After  string `json:"after,omitempty" jsonschema:"Cursor for pagination"`
	Filter string `json:"filter,omitempty" jsonschema:"HTTPQL filter query"`
}

// InterceptEntrySummary is a minimal intercept entry
type InterceptEntrySummary struct {
	ID         string `json:"id"`
	RequestID  string `json:"requestId"`
	Method     string `json:"method"`
	URL        string `json:"url"`
	StatusCode int    `json:"statusCode,omitempty"`
	CreatedAt  string `json:"createdAt"`
}

// ListInterceptEntriesOutput is the output
type ListInterceptEntriesOutput struct {
	Entries    []InterceptEntrySummary `json:"entries"`
	HasMore    bool                    `json:"hasMore"`
	NextCursor string                  `json:"nextCursor,omitempty"`
	Total      int                     `json:"total"`
}

func listInterceptEntriesHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, ListInterceptEntriesInput) (*mcp.CallToolResult, ListInterceptEntriesOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input ListInterceptEntriesInput,
	) (*mcp.CallToolResult, ListInterceptEntriesOutput, error) {
		if len(input.Filter) > 10000 {
			return nil, ListInterceptEntriesOutput{}, fmt.Errorf(
				"filter exceeds max length of 10000",
			)
		}

		limit := input.Limit
		if limit <= 0 {
			limit = 20
		}
		if limit > 100 {
			limit = 100
		}

		opts := &caido.ListInterceptEntriesOptions{
			First: &limit,
		}
		if input.Filter != "" {
			opts.Filter = &input.Filter
		}
		if input.After != "" {
			opts.After = &input.After
		}

		resp, err := client.Intercept.ListEntries(ctx, opts)
		if err != nil {
			return nil, ListInterceptEntriesOutput{}, fmt.Errorf(
				"failed to list intercept entries: %w", err,
			)
		}

		conn := resp.InterceptEntries
		output := ListInterceptEntriesOutput{
			Entries: make(
				[]InterceptEntrySummary, 0, len(conn.Edges),
			),
		}

		output.Total = conn.Count.Value

		for _, edge := range conn.Edges {
			e := edge.Node
			r := e.Request
			summary := InterceptEntrySummary{
				ID:        e.Id,
				RequestID: r.Id,
				Method:    r.Method,
				URL: httputil.BuildURL(
					r.IsTls, r.Host, r.Port, r.Path, r.Query,
				),
				CreatedAt: time.UnixMilli(r.CreatedAt).Format(
					time.RFC3339,
				),
			}
			if r.Response != nil {
				summary.StatusCode = r.Response.StatusCode
			}
			output.Entries = append(output.Entries, summary)
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

// RegisterListInterceptEntriesTool registers the tool
func RegisterListInterceptEntriesTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_list_intercept_entries",
		Description: `List queued intercept entries. Filter with httpql. Returns id/method/url/status. Use with forward/drop tools.`,
	}, listInterceptEntriesHandler(client))
}
