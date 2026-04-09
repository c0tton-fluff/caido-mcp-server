package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	gen "github.com/caido-community/sdk-go/graphql"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RunWorkflowInput is the input for the run_workflow tool
type RunWorkflowInput struct {
	ID        string  `json:"id" jsonschema:"required,Workflow ID"`
	Type      string  `json:"type" jsonschema:"required,Workflow type: active or convert"`
	RequestID *string `json:"request_id,omitempty" jsonschema:"Request ID (required for active workflows)"`
	Input     *string `json:"input,omitempty" jsonschema:"Input data as string (required for convert workflows)"`
}

// RunWorkflowOutput is the output of the run_workflow tool
type RunWorkflowOutput struct {
	TaskID *string `json:"task_id,omitempty"`
	Output *string `json:"output,omitempty"`
}

// runWorkflowHandler creates the handler function
func runWorkflowHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, RunWorkflowInput) (*mcp.CallToolResult, RunWorkflowOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input RunWorkflowInput,
	) (*mcp.CallToolResult, RunWorkflowOutput, error) {
		if input.Input != nil && len(*input.Input) > 1048576 {
			return nil, RunWorkflowOutput{}, fmt.Errorf(
				"input exceeds max length of 1MB",
			)
		}

		switch input.Type {
		case "active":
			if input.RequestID == nil {
				return nil, RunWorkflowOutput{}, fmt.Errorf(
					"request_id is required for active workflows",
				)
			}

			activeInput := &gen.RunActiveWorkflowInput{
				RequestId: *input.RequestID,
			}
			resp, err := client.Workflows.RunActive(
				ctx, input.ID, activeInput,
			)
			if err != nil {
				return nil, RunWorkflowOutput{}, err
			}

			payload := resp.RunActiveWorkflow
			if payload.Error != nil {
				errType := "unknown"
				if tn := (*payload.Error).GetTypename(); tn != nil {
					errType = *tn
				}
				return nil, RunWorkflowOutput{}, fmt.Errorf(
					"run active workflow failed: %s", errType,
				)
			}

			var taskID *string
			if payload.Task != nil {
				taskID = &payload.Task.Id
			}

			return nil, RunWorkflowOutput{
				TaskID: taskID,
			}, nil

		case "convert":
			if input.Input == nil {
				return nil, RunWorkflowOutput{}, fmt.Errorf(
					"input is required for convert workflows",
				)
			}

			resp, err := client.Workflows.RunConvert(
				ctx, input.ID, *input.Input,
			)
			if err != nil {
				return nil, RunWorkflowOutput{}, err
			}

			payload := resp.RunConvertWorkflow
			if payload.Error != nil {
				errType := "unknown"
				if tn := (*payload.Error).GetTypename(); tn != nil {
					errType = *tn
				}
				return nil, RunWorkflowOutput{}, fmt.Errorf(
					"run convert workflow failed: %s", errType,
				)
			}

			return nil, RunWorkflowOutput{
				Output: payload.Output,
			}, nil

		default:
			return nil, RunWorkflowOutput{}, fmt.Errorf(
				"type must be 'active' or 'convert'",
			)
		}
	}
}

// RegisterRunWorkflowTool registers the tool
func RegisterRunWorkflowTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "caido_run_workflow",
		Description: `Execute a workflow. Params: id (required), ` +
			`type (active/convert), request_id (for active), ` +
			`input (for convert). Active workflows run on a ` +
			`request and return a task_id. Convert workflows ` +
			`transform input data and return the output.`,
	}, runWorkflowHandler(client))
}
