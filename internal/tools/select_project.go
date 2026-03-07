package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// SelectProjectInput is the input for the select_project tool
type SelectProjectInput struct {
	ID string `json:"id" jsonschema:"required,Project ID to switch to"`
}

// SelectProjectOutput is the output of the select_project tool
type SelectProjectOutput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// selectProjectHandler creates the handler function
func selectProjectHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, SelectProjectInput) (*mcp.CallToolResult, SelectProjectOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input SelectProjectInput,
	) (*mcp.CallToolResult, SelectProjectOutput, error) {
		if input.ID == "" {
			return nil, SelectProjectOutput{}, fmt.Errorf(
				"project ID is required",
			)
		}

		resp, err := client.Projects.Select(ctx, input.ID)
		if err != nil {
			return nil, SelectProjectOutput{}, err
		}

		payload := resp.SelectProject
		if payload.Error != nil || payload.CurrentProject == nil {
			return nil, SelectProjectOutput{}, fmt.Errorf(
				"failed to select project",
			)
		}

		cp := payload.CurrentProject
		if cp.Project == nil {
			return nil, SelectProjectOutput{}, fmt.Errorf(
				"no project returned after select",
			)
		}
		return nil, SelectProjectOutput{
			ID:   cp.Project.Id,
			Name: cp.Project.Name,
		}, nil
	}
}

// RegisterSelectProjectTool registers the tool
func RegisterSelectProjectTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_select_project",
		Description: `Switch active project. Params: id.`,
	}, selectProjectHandler(client))
}
