package tools

import (
	"context"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GetInstanceInput is the input for the get_instance tool
type GetInstanceInput struct{}

// GetInstanceOutput is the output of the get_instance tool
type GetInstanceOutput struct {
	Version  string `json:"version"`
	Platform string `json:"platform"`
}

// getInstanceHandler creates the handler function
func getInstanceHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, GetInstanceInput) (*mcp.CallToolResult, GetInstanceOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input GetInstanceInput,
	) (*mcp.CallToolResult, GetInstanceOutput, error) {
		resp, err := client.Instance.GetRuntime(ctx)
		if err != nil {
			return nil, GetInstanceOutput{}, err
		}

		r := resp.Runtime
		return nil, GetInstanceOutput{
			Version:  r.Version,
			Platform: r.Platform,
		}, nil
	}
}

// RegisterGetInstanceTool registers the tool
func RegisterGetInstanceTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "caido_get_instance",
		Description: `Get Caido instance info. ` +
			`Returns version and platform.`,
	}, getInstanceHandler(client))
}
