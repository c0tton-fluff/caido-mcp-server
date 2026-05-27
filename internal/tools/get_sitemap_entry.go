package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type GetSitemapEntryInput struct {
	ID string `json:"id" jsonschema:"required,Sitemap entry ID"`
}

type GetSitemapEntryOutput struct {
	ID             string  `json:"id"`
	Label          string  `json:"label"`
	Kind           string  `json:"kind"`
	ParentID       *string `json:"parentId,omitempty"`
	HasDescendants bool    `json:"hasDescendants"`
}

func getSitemapEntryHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, GetSitemapEntryInput) (*mcp.CallToolResult, GetSitemapEntryOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input GetSitemapEntryInput,
	) (*mcp.CallToolResult, GetSitemapEntryOutput, error) {
		resp, err := client.Sitemap.GetEntry(ctx, input.ID)
		if err != nil {
			return nil, GetSitemapEntryOutput{}, err
		}

		e := resp.SitemapEntry
		if e == nil {
			return nil, GetSitemapEntryOutput{}, fmt.Errorf("sitemap entry not found")
		}

		return nil, GetSitemapEntryOutput{
			ID:             e.Id,
			Label:          e.Label,
			Kind:           string(e.Kind),
			ParentID:       e.ParentId,
			HasDescendants: e.HasDescendants,
		}, nil
	}
}

func RegisterGetSitemapEntryTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "caido_get_sitemap_entry",
		Description: `Get a single sitemap entry by ID (label, kind, parent, whether ` +
			`it has children). Use caido_get_sitemap to browse the tree.`,
	}, getSitemapEntryHandler(client))
}
