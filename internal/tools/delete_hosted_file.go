package tools

import (
	"context"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type DeleteHostedFileInput struct {
	ID string `json:"id" jsonschema:"required,Hosted file ID to delete"`
}

type DeleteHostedFileOutput struct {
	Success   bool   `json:"success"`
	DeletedID string `json:"deletedId,omitempty"`
}

func deleteHostedFileHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, DeleteHostedFileInput) (*mcp.CallToolResult, DeleteHostedFileOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input DeleteHostedFileInput,
	) (*mcp.CallToolResult, DeleteHostedFileOutput, error) {
		resp, err := client.HostedFiles.Delete(ctx, input.ID)
		if err != nil {
			return nil, DeleteHostedFileOutput{}, err
		}

		out := DeleteHostedFileOutput{Success: true}
		if id := resp.DeleteHostedFile.DeletedId; id != nil {
			out.DeletedID = *id
		}
		return nil, out, nil
	}
}

func RegisterDeleteHostedFileTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_delete_hosted_file",
		Description: `Delete a hosted file by ID.`,
	}, deleteHostedFileHandler(client))
}
