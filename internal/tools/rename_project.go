package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type RenameProjectInput struct {
	ID   string `json:"id" jsonschema:"required,Project ID to rename"`
	Name string `json:"name" jsonschema:"required,New project name"`
}

type RenameProjectOutput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func renameProjectHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, RenameProjectInput) (*mcp.CallToolResult, RenameProjectOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input RenameProjectInput,
	) (*mcp.CallToolResult, RenameProjectOutput, error) {
		if input.ID == "" {
			return nil, RenameProjectOutput{}, fmt.Errorf(
				"project ID is required",
			)
		}
		if input.Name == "" {
			return nil, RenameProjectOutput{}, fmt.Errorf(
				"project name is required",
			)
		}

		resp, err := client.Projects.Rename(ctx, input.ID, input.Name)
		if err != nil {
			return nil, RenameProjectOutput{}, fmt.Errorf("rename project: %w", err)
		}

		project := resp.RenameProject.Project
		if project == nil {
			return nil, RenameProjectOutput{}, fmt.Errorf("rename project returned nil")
		}

		return nil, RenameProjectOutput{
			ID:   project.Id,
			Name: project.Name,
		}, nil
	}
}

func RegisterRenameProjectTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_rename_project",
		Description: `Rename an existing project.`,
		Annotations: writeAnn(false, true, false),
	}, renameProjectHandler(client))
}
