package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type DeleteProjectInput struct {
	ID string `json:"id" jsonschema:"required,Project ID to delete"`
}

type DeleteProjectOutput struct {
	Success bool `json:"success"`
}

func deleteProjectHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, DeleteProjectInput) (*mcp.CallToolResult, DeleteProjectOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input DeleteProjectInput,
	) (*mcp.CallToolResult, DeleteProjectOutput, error) {
		if input.ID == "" {
			return nil, DeleteProjectOutput{}, fmt.Errorf(
				"project ID is required",
			)
		}

		_, err := client.Projects.Delete(ctx, input.ID)
		if err != nil {
			return nil, DeleteProjectOutput{}, fmt.Errorf("delete project: %w", err)
		}

		return nil, DeleteProjectOutput{
			Success: true,
		}, nil
	}
}

func RegisterDeleteProjectTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_delete_project",
		Description: `Delete a project by ID. This operation cannot be undone.`,
		Annotations: writeAnn(true, true, false),
	}, deleteProjectHandler(client))
}
