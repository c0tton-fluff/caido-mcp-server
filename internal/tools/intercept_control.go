package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// InterceptControlInput is the input for the intercept_control tool
type InterceptControlInput struct {
	Action string `json:"action" jsonschema:"required,Action: pause or resume"`
}

// InterceptControlOutput is the output
type InterceptControlOutput struct {
	Action string `json:"action"`
	Status string `json:"status"`
}

// interceptControlHandler creates the handler function
func interceptControlHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, InterceptControlInput) (*mcp.CallToolResult, InterceptControlOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input InterceptControlInput,
	) (*mcp.CallToolResult, InterceptControlOutput, error) {
		switch input.Action {
		case "pause":
			_, err := client.Intercept.Pause(ctx)
			if err != nil {
				return nil, InterceptControlOutput{}, err
			}
			return nil, InterceptControlOutput{
				Action: "pause",
				Status: "PAUSED",
			}, nil

		case "resume":
			_, err := client.Intercept.Resume(ctx)
			if err != nil {
				return nil, InterceptControlOutput{}, err
			}
			return nil, InterceptControlOutput{
				Action: "resume",
				Status: "RUNNING",
			}, nil

		default:
			return nil, InterceptControlOutput{}, fmt.Errorf(
				"action must be 'pause' or 'resume'",
			)
		}
	}
}

// RegisterInterceptControlTool registers the tool
func RegisterInterceptControlTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_intercept_control",
		Description: `Toggle intercept. Params: action (pause/resume).`,
	}, interceptControlHandler(client))
}
