package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	gen "github.com/caido-community/sdk-go/graphql"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type CreateProjectInput struct {
	Name string `json:"name" jsonschema:"required,Project name"`
}

type CreateProjectOutput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func createProjectHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, CreateProjectInput) (*mcp.CallToolResult, CreateProjectOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input CreateProjectInput,
	) (*mcp.CallToolResult, CreateProjectOutput, error) {
		if input.Name == "" {
			return nil, CreateProjectOutput{}, fmt.Errorf(
				"project name is required",
			)
		}

		createInput := &gen.CreateProjectInput{
			Name: input.Name,
		}

		resp, err := client.Projects.Create(ctx, createInput)
		if err != nil {
			return nil, CreateProjectOutput{}, fmt.Errorf("create project: %w", err)
		}

		project := resp.CreateProject.Project
		if project == nil {
			return nil, CreateProjectOutput{}, fmt.Errorf("create project returned nil")
		}

		return nil, CreateProjectOutput{
			ID:   project.Id,
			Name: project.Name,
		}, nil
	}
}

func RegisterCreateProjectTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_create_project",
		Description: `Create a new project with the specified name.`,
		Annotations: writeAnn(false, false, false),
	}, createProjectHandler(client))
}
