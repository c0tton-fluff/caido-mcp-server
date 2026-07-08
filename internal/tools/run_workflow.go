package tools

import (
	"context"
	"encoding/base64"
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
		if input.Input != nil {
			if err := checkRawSize("input", *input.Input); err != nil {
				return nil, RunWorkflowOutput{}, err
			}
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

			// Blob type requires base64 encoding.
			encoded := base64.StdEncoding.EncodeToString(
				[]byte(*input.Input),
			)

			resp, err := client.Workflows.RunConvert(
				ctx, input.ID, encoded,
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

			// Decode base64 output if present.
			var output *string
			if payload.Output != nil {
				decoded, decErr := base64.StdEncoding.DecodeString(
					*payload.Output,
				)
				if decErr != nil {
					// Not base64 -- return as-is.
					output = payload.Output
				} else {
					s := string(decoded)
					output = &s
				}
			}

			return nil, RunWorkflowOutput{
				Output: output,
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
