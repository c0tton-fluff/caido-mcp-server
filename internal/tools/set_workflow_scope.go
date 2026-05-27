package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type SetWorkflowScopeInput struct {
	ID     string `json:"id" jsonschema:"required,Workflow ID"`
	Global bool   `json:"global" jsonschema:"required,true to globalize (share across projects), false to localize to the current project"`
}

type SetWorkflowScopeOutput struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Global bool   `json:"global"`
}

func setWorkflowScopeHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, SetWorkflowScopeInput) (*mcp.CallToolResult, SetWorkflowScopeOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input SetWorkflowScopeInput,
	) (*mcp.CallToolResult, SetWorkflowScopeOutput, error) {
		if input.Global {
			resp, err := client.Workflows.Globalize(ctx, input.ID)
			if err != nil {
				return nil, SetWorkflowScopeOutput{}, err
			}
			payload := resp.GlobalizeWorkflow
			if payload.Error != nil {
				errType := "unknown"
				if tn := (*payload.Error).GetTypename(); tn != nil {
					errType = *tn
				}
				return nil, SetWorkflowScopeOutput{}, fmt.Errorf("globalize workflow failed: %s", errType)
			}
			if payload.Workflow == nil {
				return nil, SetWorkflowScopeOutput{}, fmt.Errorf("globalize workflow returned no workflow")
			}
			return nil, SetWorkflowScopeOutput{
				ID:     payload.Workflow.Id,
				Name:   payload.Workflow.Name,
				Global: payload.Workflow.Global,
			}, nil
		}

		resp, err := client.Workflows.Localize(ctx, input.ID)
		if err != nil {
			return nil, SetWorkflowScopeOutput{}, err
		}
		payload := resp.LocalizeWorkflow
		if payload.Error != nil {
			errType := "unknown"
			if tn := (*payload.Error).GetTypename(); tn != nil {
				errType = *tn
			}
			return nil, SetWorkflowScopeOutput{}, fmt.Errorf("localize workflow failed: %s", errType)
		}
		if payload.Workflow == nil {
			return nil, SetWorkflowScopeOutput{}, fmt.Errorf("localize workflow returned no workflow")
		}
		return nil, SetWorkflowScopeOutput{
			ID:     payload.Workflow.Id,
			Name:   payload.Workflow.Name,
			Global: payload.Workflow.Global,
		}, nil
	}
}

func RegisterSetWorkflowScopeTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "caido_set_workflow_scope",
		Description: `Change a workflow's scope: globalize (global=true) to share it ` +
			`across all projects, or localize (global=false) to bind it to the ` +
			`current project.`,
	}, setWorkflowScopeHandler(client))
}
