package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type DeleteWorkflowInput struct {
	ID string `json:"id" jsonschema:"required,Workflow ID to delete"`
}

type DeleteWorkflowOutput struct {
	Success   bool   `json:"success"`
	DeletedID string `json:"deletedId,omitempty"`
}

func deleteWorkflowHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, DeleteWorkflowInput) (*mcp.CallToolResult, DeleteWorkflowOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input DeleteWorkflowInput,
	) (*mcp.CallToolResult, DeleteWorkflowOutput, error) {
		resp, err := client.Workflows.Delete(ctx, input.ID)
		if err != nil {
			return nil, DeleteWorkflowOutput{}, err
		}

		payload := resp.DeleteWorkflow
		if payload.Error != nil {
			errType := "unknown"
			if tn := (*payload.Error).GetTypename(); tn != nil {
				errType = *tn
			}
			return nil, DeleteWorkflowOutput{}, fmt.Errorf("delete workflow failed: %s", errType)
		}

		out := DeleteWorkflowOutput{Success: true}
		if payload.DeletedId != nil {
			out.DeletedID = *payload.DeletedId
		}
		return nil, out, nil
	}
}

func RegisterDeleteWorkflowTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_delete_workflow",
		Description: `Delete a workflow.`,
	}, deleteWorkflowHandler(client))
}
