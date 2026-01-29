package tools

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/c0tton-fluff/caido-mcp-server/internal/caido"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GetAutomateEntryInput is the input for the get_automate_entry tool
type GetAutomateEntryInput struct {
	ID    string `json:"id" jsonschema:"required,Automate entry ID"`
	Limit int    `json:"limit,omitempty" jsonschema:"Maximum number of fuzz results to return (default 50, max 100)"`
	After string `json:"after,omitempty" jsonschema:"Cursor for pagination from previous response nextCursor"`
}

// FuzzRequestResult represents a single fuzz result
type FuzzRequestResult struct {
	SequenceID  string   `json:"sequenceId"`
	Payloads    []string `json:"payloads"` // Decoded payloads
	Error       *string  `json:"error,omitempty"`
	RequestID   string   `json:"requestId"`
	Method      string   `json:"method"`
	URL         string   `json:"url"`
	StatusCode  int      `json:"statusCode,omitempty"`
	RoundtripMs int      `json:"roundtripMs,omitempty"`
}

// GetAutomateEntryOutput is the output of the get_automate_entry tool
type GetAutomateEntryOutput struct {
	ID         string              `json:"id"`
	Name       string              `json:"name"`
	CreatedAt  string              `json:"createdAt"`
	Results    []FuzzRequestResult `json:"results"`
	HasMore    bool                `json:"hasMore"`
	NextCursor string              `json:"nextCursor,omitempty"`
}

// getAutomateEntryHandler creates the handler function for the get_automate_entry tool
func getAutomateEntryHandler(client *caido.Client) func(context.Context, *mcp.CallToolRequest, GetAutomateEntryInput) (*mcp.CallToolResult, GetAutomateEntryOutput, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input GetAutomateEntryInput) (*mcp.CallToolResult, GetAutomateEntryOutput, error) {
		if input.ID == "" {
			return nil, GetAutomateEntryOutput{}, fmt.Errorf("entry ID is required")
		}

		limit := input.Limit
		if limit <= 0 {
			limit = 10 // Small default to save context
		}
		if limit > 100 {
			limit = 100
		}

		opts := caido.GetAutomateEntryOptions{
			First: limit,
			After: input.After,
		}

		entry, err := client.GetAutomateEntry(ctx, input.ID, opts)
		if err != nil {
			return nil, GetAutomateEntryOutput{}, err
		}

		output := GetAutomateEntryOutput{
			ID:        entry.ID,
			Name:      entry.Name,
			CreatedAt: entry.CreatedAt.Time().Format(time.RFC3339),
			Results:   make([]FuzzRequestResult, 0),
		}

		if entry.Requests != nil {
			output.HasMore = entry.Requests.PageInfo.HasNextPage
			output.NextCursor = entry.Requests.PageInfo.EndCursor

			for _, edge := range entry.Requests.Edges {
				r := edge.Node

				// Decode payloads
				payloads := make([]string, 0, len(r.Payloads))
				for _, p := range r.Payloads {
					decoded, err := base64.StdEncoding.DecodeString(p.Raw)
					if err == nil {
						payloads = append(payloads, string(decoded))
					} else {
						payloads = append(payloads, p.Raw) // fallback to raw
					}
				}

				result := FuzzRequestResult{
					SequenceID: r.SequenceID,
					Payloads:   payloads,
					Error:      r.Error,
				}

				if r.Request != nil {
					result.RequestID = r.Request.ID
					result.Method = r.Request.Method
					result.URL = buildURL(r.Request.IsTLS, r.Request.Host, r.Request.Port, r.Request.Path, r.Request.Query)
					if r.Request.Response != nil {
						result.StatusCode = r.Request.Response.StatusCode
						result.RoundtripMs = r.Request.Response.RoundtripTime
					}
				}

				output.Results = append(output.Results, result)
			}
		}

		return nil, output, nil
	}
}

// RegisterGetAutomateEntryTool registers the tool with the MCP server
func RegisterGetAutomateEntryTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_get_automate_entry",
		Description: `Get fuzz results. Returns sequenceId/payloads/requestId/statusCode. Use limit/after for pagination.`,
	}, getAutomateEntryHandler(client))
}
