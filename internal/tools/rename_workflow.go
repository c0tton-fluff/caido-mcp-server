package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type RenameWorkflowInput struct {
	ID   string `json:"id" jsonschema:"required,Workflow ID"`
	Name string `json:"name" jsonschema:"required,New workflow name"`
}

type RenameWorkflowOutput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func renameWorkflowHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, RenameWorkflowInput) (*mcp.CallToolResult, RenameWorkflowOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input RenameWorkflowInput,
	) (*mcp.CallToolResult, RenameWorkflowOutput, error) {
		resp, err := client.Workflows.Rename(ctx, input.ID, input.Name)
		if err != nil {
			return nil, RenameWorkflowOutput{}, err
		}

		payload := resp.RenameWorkflow
		if payload.Error != nil {
			errType := "unknown"
			if tn := (*payload.Error).GetTypename(); tn != nil {
				errType = *tn
			}
			return nil, RenameWorkflowOutput{}, fmt.Errorf("rename workflow failed: %s", errType)
		}
		if payload.Workflow == nil {
			return nil, RenameWorkflowOutput{}, fmt.Errorf("rename workflow returned no workflow")
		}

		return nil, RenameWorkflowOutput{
			ID:   payload.Workflow.Id,
			Name: payload.Workflow.Name,
		}, nil
	}
}

func RegisterRenameWorkflowTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_rename_workflow",
		Description: `Rename a workflow.`,
	}, renameWorkflowHandler(client))
}
