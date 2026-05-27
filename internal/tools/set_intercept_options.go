package tools

import (
	"context"

	caido "github.com/caido-community/sdk-go"
	gen "github.com/caido-community/sdk-go/graphql"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type SetInterceptOptionsInput struct {
	RequestEnabled  bool    `json:"requestEnabled" jsonschema:"required,Intercept requests"`
	ResponseEnabled bool    `json:"responseEnabled" jsonschema:"required,Intercept responses"`
	StreamWsEnabled bool    `json:"streamWsEnabled,omitempty" jsonschema:"Intercept WebSocket streams (default false)"`
	ScopeID         *string `json:"scopeId,omitempty" jsonschema:"Restrict interception to this scope ID (omit for all traffic)"`
}

type SetInterceptOptionsOutput struct {
	RequestEnabled  bool `json:"requestEnabled"`
	ResponseEnabled bool `json:"responseEnabled"`
}

func setInterceptOptionsHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, SetInterceptOptionsInput) (*mcp.CallToolResult, SetInterceptOptionsOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input SetInterceptOptionsInput,
	) (*mcp.CallToolResult, SetInterceptOptionsOutput, error) {
		optsInput := gen.InterceptOptionsInput{
			Request: gen.InterceptRequestOptionsInput{
				Enabled: input.RequestEnabled,
			},
			Response: gen.InterceptResponseOptionsInput{
				Enabled: input.ResponseEnabled,
			},
			StreamWs: gen.InterceptStreamWsOptionsInput{
				Enabled: input.StreamWsEnabled,
			},
		}
		if input.ScopeID != nil && *input.ScopeID != "" {
			optsInput.Scope = &gen.InterceptScopeOptionsInput{
				ScopeId: *input.ScopeID,
			}
		}

		resp, err := client.Intercept.SetOptions(ctx, &optsInput)
		if err != nil {
			return nil, SetInterceptOptionsOutput{}, err
		}

		opts := resp.SetInterceptOptions.Options
		return nil, SetInterceptOptionsOutput{
			RequestEnabled:  opts.Request.Enabled,
			ResponseEnabled: opts.Response.Enabled,
		}, nil
	}
}

func RegisterSetInterceptOptionsTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "caido_set_intercept_options",
		Description: `Configure the intercept proxy: toggle request/response/WebSocket ` +
			`interception and optionally restrict it to a scope. Note: this does not ` +
			`pause/resume interception itself (use caido_intercept_control).`,
	}, setInterceptOptionsHandler(client))
}
