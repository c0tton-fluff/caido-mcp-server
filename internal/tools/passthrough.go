package tools

import (
	"context"
	"fmt"

	gql "github.com/Khan/genqlient/graphql"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Pass-through options (Caido 0.59+) decide which TLS traffic Caido tunnels
// without intercepting it. sdk-go has no operations for them, and the set
// mutation requires every field, so updates read the current options first.
//
// Verified against a live 0.59.0 server: a host on passthroughList is
// tunneled (the client sees the origin's certificate) and never reaches
// history; outOfScope:true does the same for every host outside the scope.
// captureList is preserved but not settable: a bare host on it changed
// neither behaviour in testing, host:port is rejected, and Caido does not
// document its precedence.

const getPassthroughQuery = `query GetPassthroughOptions {
  passthroughOptions { captureList passthroughList outOfScope }
}`

const setPassthroughMutation = `mutation SetPassthroughOptions($input: SetPassthroughOptionsInput) {
  setPassthroughOptions(input: $input) {
    options { captureList passthroughList outOfScope }
  }
}`

// PassthroughOptions is the pass-through configuration.
type PassthroughOptions struct {
	CaptureList     []string `json:"captureList"`
	PassthroughList []string `json:"passthroughList"`
	OutOfScope      bool     `json:"outOfScope"`
}

// normalized replaces nil lists with empty ones: the set mutation takes
// non-null lists, and callers read [] more easily than null.
func (o PassthroughOptions) normalized() PassthroughOptions {
	if o.CaptureList == nil {
		o.CaptureList = []string{}
	}
	if o.PassthroughList == nil {
		o.PassthroughList = []string{}
	}
	return o
}

func getPassthrough(ctx context.Context, client *caido.Client) (PassthroughOptions, error) {
	var data struct {
		PassthroughOptions *PassthroughOptions `json:"passthroughOptions"`
	}
	if err := client.GraphQL.MakeRequest(ctx, &gql.Request{
		OpName: "GetPassthroughOptions",
		Query:  getPassthroughQuery,
	}, &gql.Response{Data: &data}); err != nil {
		return PassthroughOptions{}, fmt.Errorf(
			"get pass-through options (needs Caido 0.59+): %w", err,
		)
	}
	if data.PassthroughOptions == nil {
		return PassthroughOptions{}.normalized(), nil
	}
	return data.PassthroughOptions.normalized(), nil
}

// PassthroughInput updates pass-through options. Omitted fields keep their
// current value; with no fields the call only reads.
type PassthroughInput struct {
	PassthroughList []string `json:"passthroughList,omitempty" jsonschema:"Replace the list of hosts (no port) whose TLS is tunneled without interception or recording, e.g. a pinned or noisy third party"`
	OutOfScope      *bool    `json:"outOfScope,omitempty" jsonschema:"Tunnel TLS for every host outside the current scope, so history holds only in-scope traffic"`
}

// PassthroughOutput reports the options in effect after the call.
type PassthroughOutput struct {
	Changed bool               `json:"changed"`
	Options PassthroughOptions `json:"options"`
}

func passthroughHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, PassthroughInput) (*mcp.CallToolResult, PassthroughOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input PassthroughInput,
	) (*mcp.CallToolResult, PassthroughOutput, error) {
		current, err := getPassthrough(ctx, client)
		if err != nil {
			return nil, PassthroughOutput{}, err
		}
		if input.PassthroughList == nil && input.OutOfScope == nil {
			return nil, PassthroughOutput{Options: current}, nil
		}

		next := current
		if input.PassthroughList != nil {
			next.PassthroughList = input.PassthroughList
		}
		if input.OutOfScope != nil {
			next.OutOfScope = *input.OutOfScope
		}
		next = next.normalized()

		var data struct {
			SetPassthroughOptions struct {
				Options *PassthroughOptions `json:"options"`
			} `json:"setPassthroughOptions"`
		}
		if err := client.GraphQL.MakeRequest(ctx, &gql.Request{
			OpName: "SetPassthroughOptions",
			Query:  setPassthroughMutation,
			Variables: map[string]any{"input": map[string]any{
				"captureList":     next.CaptureList,
				"passthroughList": next.PassthroughList,
				"outOfScope":      next.OutOfScope,
			}},
		}, &gql.Response{Data: &data}); err != nil {
			return nil, PassthroughOutput{}, fmt.Errorf("set pass-through options: %w", err)
		}
		applied := next
		if data.SetPassthroughOptions.Options != nil {
			applied = data.SetPassthroughOptions.Options.normalized()
		}
		return nil, PassthroughOutput{Changed: true, Options: applied}, nil
	}
}

// RegisterPassthroughTool registers the passthrough_options tool.
func RegisterPassthroughTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:  "caido_passthrough_options",
		Title: "TLS Pass-through Options",
		Description: `Read or update Caido's TLS pass-through options (Caido 0.59+): hosts whose TLS Caido tunnels without intercepting, so their traffic is not decrypted or recorded. ` +
			`Call with no arguments to read. passthroughList replaces the whole list (hosts, no port); omitted fields are left as they are. ` +
			`outOfScope:true tunnels everything outside the current scope.`,
		Annotations: writeAnn(false, true, false),
	}, passthroughHandler(client))
}
