package tools

import (
	"context"
	"time"

	"github.com/c0tton-fluff/caido-mcp-server/internal/caido"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ListFindingsInput is the input for the list_findings tool
type ListFindingsInput struct {
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum number of findings to return (default 50)"`
	After  string `json:"after,omitempty" jsonschema:"Cursor for pagination"`
	Filter string `json:"filter,omitempty" jsonschema:"HTTPQL filter query"`
}

// FindingSummary is a summary of a finding
type FindingSummary struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Host        string  `json:"host"`
	Path        string  `json:"path"`
	Reporter    string  `json:"reporter"`
	CreatedAt   string  `json:"createdAt"`
	RequestID   string  `json:"requestId,omitempty"`
	Description *string `json:"description,omitempty"`
}

// ListFindingsOutput is the output of the list_findings tool
type ListFindingsOutput struct {
	Findings   []FindingSummary `json:"findings"`
	HasMore    bool             `json:"hasMore"`
	NextCursor string           `json:"nextCursor,omitempty"`
}

// listFindingsHandler creates the handler function
func listFindingsHandler(client *caido.Client) func(context.Context, *mcp.CallToolRequest, ListFindingsInput) (*mcp.CallToolResult, ListFindingsOutput, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input ListFindingsInput) (*mcp.CallToolResult, ListFindingsOutput, error) {
		limit := input.Limit
		if limit <= 0 {
			limit = 10 // Small default to save context
		}
		if limit > 100 {
			limit = 100
		}

		opts := caido.ListFindingsOptions{
			First:  limit,
			After:  input.After,
			Filter: input.Filter,
		}

		result, err := client.ListFindings(ctx, opts)
		if err != nil {
			return nil, ListFindingsOutput{}, err
		}

		output := ListFindingsOutput{
			Findings:   make([]FindingSummary, 0, len(result.Findings.Edges)),
			HasMore:    result.Findings.PageInfo.HasNextPage,
			NextCursor: result.Findings.PageInfo.EndCursor,
		}

		for _, edge := range result.Findings.Edges {
			f := edge.Node
			summary := FindingSummary{
				ID:          f.ID,
				Title:       f.Title,
				Host:        f.Host,
				Path:        f.Path,
				Reporter:    f.Reporter,
				CreatedAt:   f.CreatedAt.Time().Format(time.RFC3339),
				Description: f.Description,
			}
			if f.Request != nil {
				summary.RequestID = f.Request.ID
			}
			output.Findings = append(output.Findings, summary)
		}

		return nil, output, nil
	}
}

// RegisterListFindingsTool registers the tool with the MCP server
func RegisterListFindingsTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_list_findings",
		Description: `List security findings. Returns title/host/path/requestId.`,
	}, listFindingsHandler(client))
}
