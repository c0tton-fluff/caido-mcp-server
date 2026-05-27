package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type RenameHostedFileInput struct {
	ID   string `json:"id" jsonschema:"required,Hosted file ID"`
	Name string `json:"name" jsonschema:"required,New file name"`
}

type RenameHostedFileOutput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func renameHostedFileHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, RenameHostedFileInput) (*mcp.CallToolResult, RenameHostedFileOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input RenameHostedFileInput,
	) (*mcp.CallToolResult, RenameHostedFileOutput, error) {
		resp, err := client.HostedFiles.Rename(ctx, input.ID, input.Name)
		if err != nil {
			return nil, RenameHostedFileOutput{}, err
		}

		file := resp.RenameHostedFile.HostedFile
		if file == nil {
			return nil, RenameHostedFileOutput{}, fmt.Errorf("rename hosted file returned nil")
		}

		return nil, RenameHostedFileOutput{
			ID:   file.Id,
			Name: file.Name,
		}, nil
	}
}

func RegisterRenameHostedFileTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_rename_hosted_file",
		Description: `Rename a hosted file.`,
	}, renameHostedFileHandler(client))
}
