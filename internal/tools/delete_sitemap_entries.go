package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type DeleteSitemapEntriesInput struct {
	IDs []string `json:"ids" jsonschema:"required,Sitemap entry IDs to delete"`
}

type DeleteSitemapEntriesOutput struct {
	Success      bool `json:"success"`
	DeletedCount int  `json:"deletedCount"`
}

func deleteSitemapEntriesHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, DeleteSitemapEntriesInput) (*mcp.CallToolResult, DeleteSitemapEntriesOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input DeleteSitemapEntriesInput,
	) (*mcp.CallToolResult, DeleteSitemapEntriesOutput, error) {
		if len(input.IDs) == 0 {
			return nil, DeleteSitemapEntriesOutput{}, fmt.Errorf("ids is required")
		}

		resp, err := client.Sitemap.Delete(ctx, input.IDs)
		if err != nil {
			return nil, DeleteSitemapEntriesOutput{}, err
		}

		return nil, DeleteSitemapEntriesOutput{
			Success:      true,
			DeletedCount: len(resp.DeleteSitemapEntries.DeletedIds),
		}, nil
	}
}

func RegisterDeleteSitemapEntriesTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "caido_delete_sitemap_entries",
		Description: `Delete specific sitemap entries by ID. Deleting a node also ` +
			`removes its descendants.`,
	}, deleteSitemapEntriesHandler(client))
}
