package tools

import (
	"context"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type DeletePluginInput struct {
	ID string `json:"id" jsonschema:"required,Upstream plugin ID to delete"`
}

type DeletePluginOutput struct {
	Success   bool   `json:"success"`
	DeletedID string `json:"deletedId,omitempty"`
}

func deletePluginHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, DeletePluginInput) (*mcp.CallToolResult, DeletePluginOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input DeletePluginInput,
	) (*mcp.CallToolResult, DeletePluginOutput, error) {
		resp, err := client.Plugins.DeleteUpstreamPlugin(ctx, input.ID)
		if err != nil {
			return nil, DeletePluginOutput{}, err
		}

		out := DeletePluginOutput{Success: true}
		if id := resp.DeleteUpstreamPlugin.DeletedId; id != nil {
			out.DeletedID = *id
		}
		return nil, out, nil
	}
}

func RegisterDeletePluginTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_delete_plugin",
		Description: `Delete (uninstall) an upstream plugin by ID.`,
	}, deletePluginHandler(client))
}
