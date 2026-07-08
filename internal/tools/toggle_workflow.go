package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToggleWorkflowInput is the input for the toggle_workflow tool
type ToggleWorkflowInput struct {
	ID      string `json:"id" jsonschema:"required,Workflow ID"`
	Enabled bool   `json:"enabled" jsonschema:"required,Enable (true) or disable (false) the workflow"`
}

// ToggleWorkflowOutput is the output of the toggle_workflow tool
type ToggleWorkflowOutput struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Enabled bool   `json:"enabled"`
}

// toggleWorkflowHandler creates the handler function
func toggleWorkflowHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, ToggleWorkflowInput) (*mcp.CallToolResult, ToggleWorkflowOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input ToggleWorkflowInput,
	) (*mcp.CallToolResult, ToggleWorkflowOutput, error) {
		resp, err := client.Workflows.Toggle(
			ctx, input.ID, input.Enabled,
		)
		if err != nil {
			return nil, ToggleWorkflowOutput{}, err
		}

		payload := resp.ToggleWorkflow
		if payload.Error != nil {
			errType := "unknown"
			if tn := (*payload.Error).GetTypename(); tn != nil {
				errType = *tn
			}
			return nil, ToggleWorkflowOutput{}, fmt.Errorf(
				"toggle workflow failed: %s", errType,
			)
		}

		if payload.Workflow == nil {
			return nil, ToggleWorkflowOutput{}, fmt.Errorf(
				"toggle workflow returned no workflow",
			)
		}

		return nil, ToggleWorkflowOutput{
			ID:      payload.Workflow.Id,
			Name:    payload.Workflow.Name,
			Kind:    string(payload.Workflow.Kind),
			Enabled: payload.Workflow.Enabled,
		}, nil
	}
}

// RegisterToggleWorkflowTool registers the tool
func RegisterToggleWorkflowTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "caido_toggle_workflow",
		Description: `Enable or disable an automation workflow. ` +
			`Params: id (required), enabled (true/false).`,
		Annotations: writeAnn(false, false, false),
	}, toggleWorkflowHandler(client))
}
