package tools

import (
	"context"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// InterceptStatusInput is the input for the intercept_status tool
type InterceptStatusInput struct{}

// InterceptStatusOutput is the output of the intercept_status tool
type InterceptStatusOutput struct {
	Status string `json:"status"`
}

// interceptStatusHandler creates the handler function
func interceptStatusHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, InterceptStatusInput) (*mcp.CallToolResult, InterceptStatusOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input InterceptStatusInput,
	) (*mcp.CallToolResult, InterceptStatusOutput, error) {
		resp, err := client.Intercept.GetStatus(ctx)
		if err != nil {
			return nil, InterceptStatusOutput{}, err
		}

		return nil, InterceptStatusOutput{
			Status: string(resp.InterceptStatus),
		}, nil
	}
}

// RegisterInterceptStatusTool registers the tool
func RegisterInterceptStatusTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_intercept_status",
		Description: `Get intercept status (PAUSED or RUNNING).`,
	}, interceptStatusHandler(client))
}
