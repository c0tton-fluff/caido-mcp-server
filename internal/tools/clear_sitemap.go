package tools

import (
	"context"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ClearSitemapInput struct{}

type ClearSitemapOutput struct {
	Success      bool `json:"success"`
	DeletedCount int  `json:"deletedCount"`
}

func clearSitemapHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, ClearSitemapInput) (*mcp.CallToolResult, ClearSitemapOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input ClearSitemapInput,
	) (*mcp.CallToolResult, ClearSitemapOutput, error) {
		resp, err := client.Sitemap.Clear(ctx)
		if err != nil {
			return nil, ClearSitemapOutput{}, err
		}

		return nil, ClearSitemapOutput{
			Success:      true,
			DeletedCount: len(resp.ClearSitemapEntries.DeletedIds),
		}, nil
	}
}

func RegisterClearSitemapTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_clear_sitemap",
		Description: `Clear the entire sitemap tree. Destructive: removes all sitemap entries.`,
		InputSchema: map[string]any{"type": "object"},
	}, clearSitemapHandler(client))
}
