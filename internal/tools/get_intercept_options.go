package tools

import (
	"context"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type GetInterceptOptionsInput struct{}

type GetInterceptOptionsOutput struct {
	RequestEnabled  bool   `json:"requestEnabled"`
	ResponseEnabled bool   `json:"responseEnabled"`
	ScopeID         string `json:"scopeId,omitempty"`
}

func getInterceptOptionsHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, GetInterceptOptionsInput) (*mcp.CallToolResult, GetInterceptOptionsOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input GetInterceptOptionsInput,
	) (*mcp.CallToolResult, GetInterceptOptionsOutput, error) {
		resp, err := client.Intercept.GetOptions(ctx)
		if err != nil {
			return nil, GetInterceptOptionsOutput{}, err
		}

		opts := resp.InterceptOptions
		out := GetInterceptOptionsOutput{
			RequestEnabled:  opts.Request.Enabled,
			ResponseEnabled: opts.Response.Enabled,
		}
		if opts.Scope != nil {
			out.ScopeID = opts.Scope.ScopeId
		}
		return nil, out, nil
	}
}

func RegisterGetInterceptOptionsTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "caido_get_intercept_options",
		Description: `Get intercept proxy configuration: whether request/response ` +
			`interception is enabled and the active scope, if any.`,
		InputSchema: map[string]any{"type": "object"},
	}, getInterceptOptionsHandler(client))
}
