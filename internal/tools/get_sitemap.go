package tools

import (
	"context"

	caido "github.com/caido-community/sdk-go"
	gen "github.com/caido-community/sdk-go/graphql"
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
func getSitemapHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, GetSitemapInput) (*mcp.CallToolResult, GetSitemapOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input GetSitemapInput,
	) (*mcp.CallToolResult, GetSitemapOutput, error) {
		var entries []SitemapEntrySummary

		if input.ParentID == "" {
			resp, err := client.Sitemap.ListRootEntries(ctx, nil)
			if err != nil {
				return nil, GetSitemapOutput{}, err
			}

			for _, edge := range resp.SitemapRootEntries.Edges {
				e := edge.Node
				entries = append(entries, SitemapEntrySummary{
					ID:             e.Id,
					Label:          e.Label,
					Kind:           string(e.Kind),
					HasDescendants: e.HasDescendants,
				})
			}
		} else {
			resp, err := client.Sitemap.ListDescendantEntries(
				ctx, input.ParentID,
				gen.SitemapDescendantsDepthDirect,
			)
			if err != nil {
				return nil, GetSitemapOutput{}, err
			}

			for _, edge := range resp.SitemapDescendantEntries.Edges {
				e := edge.Node
				summary := SitemapEntrySummary{
					ID:             e.Id,
					Label:          e.Label,
					Kind:           string(e.Kind),
					HasDescendants: e.HasDescendants,
				}
				if e.Request != nil {
					id := e.Request.Id
					method := e.Request.Method
					summary.RequestID = &id
					summary.Method = &method
					if e.Request.Response != nil {
						sc := e.Request.Response.StatusCode
						summary.StatusCode = &sc
					}
				}
				entries = append(entries, summary)
			}
		}

		return nil, GetSitemapOutput{Entries: entries}, nil
	}
}

// RegisterGetSitemapTool registers the tool with the MCP server
func RegisterGetSitemapTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_get_sitemap",
		Description: `Get sitemap. No params=root domains. parentId=children. Returns id/label/kind.`,
	}, getSitemapHandler(client))
}
