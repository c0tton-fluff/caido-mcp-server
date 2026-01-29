package tools

import (
	"context"

	"github.com/c0tton-fluff/caido-mcp-server/internal/caido"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GetSitemapInput is the input for the get_sitemap tool
type GetSitemapInput struct {
	ParentID string `json:"parentId,omitempty" jsonschema:"Parent entry ID to get children (omit for root domains)"`
}

// SitemapEntrySummary is a summary of a sitemap entry
type SitemapEntrySummary struct {
	ID             string  `json:"id"`
	Label          string  `json:"label"`
	Kind           string  `json:"kind"`
	HasDescendants bool    `json:"hasDescendants"`
	RequestID      *string `json:"requestId,omitempty"`
	Method         *string `json:"method,omitempty"`
	StatusCode     *int    `json:"statusCode,omitempty"`
}

// GetSitemapOutput is the output of the get_sitemap tool
type GetSitemapOutput struct {
	Entries []SitemapEntrySummary `json:"entries"`
}

// getSitemapHandler creates the handler function
func getSitemapHandler(client *caido.Client) func(context.Context, *mcp.CallToolRequest, GetSitemapInput) (*mcp.CallToolResult, GetSitemapOutput, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input GetSitemapInput) (*mcp.CallToolResult, GetSitemapOutput, error) {
		var entries []SitemapEntrySummary

		if input.ParentID == "" {
			// Get root entries (domains)
			result, err := client.GetSitemapRootEntries(ctx)
			if err != nil {
				return nil, GetSitemapOutput{}, err
			}

			for _, edge := range result.SitemapRootEntries.Edges {
				e := edge.Node
				entries = append(entries, SitemapEntrySummary{
					ID:             e.ID,
					Label:          e.Label,
					Kind:           e.Kind,
					HasDescendants: e.HasDescendants,
				})
			}
		} else {
			// Get descendants of specified entry
			result, err := client.GetSitemapDescendantEntries(ctx, input.ParentID)
			if err != nil {
				return nil, GetSitemapOutput{}, err
			}

			for _, edge := range result.SitemapDescendantEntries.Edges {
				e := edge.Node
				summary := SitemapEntrySummary{
					ID:             e.ID,
					Label:          e.Label,
					Kind:           e.Kind,
					HasDescendants: e.HasDescendants,
				}
				if e.Request != nil {
					summary.RequestID = &e.Request.ID
					summary.Method = &e.Request.Method
					if e.Request.Response != nil {
						summary.StatusCode = &e.Request.Response.StatusCode
					}
				}
				entries = append(entries, summary)
			}
		}

		return nil, GetSitemapOutput{Entries: entries}, nil
	}
}

// RegisterGetSitemapTool registers the tool with the MCP server
func RegisterGetSitemapTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_get_sitemap",
		Description: `Get sitemap. No params=root domains. parentId=children. Returns id/label/kind.`,
	}, getSitemapHandler(client))
}
