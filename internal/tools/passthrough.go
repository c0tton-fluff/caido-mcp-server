package tools

import (
	"context"
	"fmt"

	gql "github.com/Khan/genqlient/graphql"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Pass-through options (Caido 0.59+) decide which traffic Caido tunnels
// without intercepting or recording it. sdk-go has no operations for them.

const getPassthroughQuery = `query GetPassthroughOptions {
  passthroughOptions { passthroughOptions { allowlist denylist outOfScope } }
}`

const setPassthroughMutation = `mutation SetPassthroughOptions($input: PassthroughOptionsInput!) {
  setPassthroughOptions(input: $input) {
    options { passthroughOptions { allowlist denylist outOfScope } }
  }
}`

// PassthroughOptions is the pass-through configuration.
type PassthroughOptions struct {
	OutOfScope bool     `json:"outOfScope"`
	Allowlist  []string `json:"allowlist"`
	Denylist   []string `json:"denylist"`
}

type passthroughWire struct {
	PassthroughOptions *struct {
		Allowlist  []string `json:"allowlist"`
		Denylist   []string `json:"denylist"`
		OutOfScope bool     `json:"outOfScope"`
	} `json:"passthroughOptions"`
}

func (w passthroughWire) options() PassthroughOptions {
	o := PassthroughOptions{Allowlist: []string{}, Denylist: []string{}}
	if p := w.PassthroughOptions; p != nil {
		o.OutOfScope = p.OutOfScope
		if p.Allowlist != nil {
			o.Allowlist = p.Allowlist
		}
		if p.Denylist != nil {
			o.Denylist = p.Denylist
		}
	}
	return o
}

func getPassthrough(ctx context.Context, client *caido.Client) (PassthroughOptions, error) {
	var data struct {
		PassthroughOptions passthroughWire `json:"passthroughOptions"`
	}
	if err := client.GraphQL.MakeRequest(ctx, &gql.Request{
		OpName: "GetPassthroughOptions",
		Query:  getPassthroughQuery,
	}, &gql.Response{Data: &data}); err != nil {
		return PassthroughOptions{}, fmt.Errorf(
			"get pass-through options (needs Caido 0.59+): %w", err,
		)
	}
	return data.PassthroughOptions.options(), nil
}

// PassthroughInput updates pass-through options. Omitted fields keep their
// current value; with no fields the call only reads.
type PassthroughInput struct {
	OutOfScope *bool    `json:"outOfScope,omitempty" jsonschema:"Pass out-of-scope traffic through without recording it, so history holds only in-scope requests"`
	Allowlist  []string `json:"allowlist,omitempty" jsonschema:"Replace the pass-through allowlist (host patterns, as in Caido's Pass-through settings)"`
	Denylist   []string `json:"denylist,omitempty" jsonschema:"Replace the pass-through denylist (host patterns, as in Caido's Pass-through settings)"`
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
		if input.OutOfScope == nil && input.Allowlist == nil && input.Denylist == nil {
			return nil, PassthroughOutput{Options: current}, nil
		}

		next := current
		if input.OutOfScope != nil {
			next.OutOfScope = *input.OutOfScope
		}
		if input.Allowlist != nil {
			next.Allowlist = input.Allowlist
		}
		if input.Denylist != nil {
			next.Denylist = input.Denylist
		}

		var data struct {
			SetPassthroughOptions struct {
				Options passthroughWire `json:"options"`
			} `json:"setPassthroughOptions"`
		}
		if err := client.GraphQL.MakeRequest(ctx, &gql.Request{
			OpName: "SetPassthroughOptions",
			Query:  setPassthroughMutation,
			Variables: map[string]any{"input": map[string]any{
				"outofscope": next.OutOfScope,
				"targets": map[string]any{
					"allowlist": next.Allowlist,
					"denylist":  next.Denylist,
				},
			}},
		}, &gql.Response{Data: &data}); err != nil {
			return nil, PassthroughOutput{}, fmt.Errorf("set pass-through options: %w", err)
		}
		return nil, PassthroughOutput{
			Changed: true,
			Options: data.SetPassthroughOptions.Options.options(),
		}, nil
	}
}

// RegisterPassthroughTool registers the passthrough_options tool.
func RegisterPassthroughTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:  "caido_passthrough_options",
		Title: "Pass-through Options",
		Description: `Read or update Caido's pass-through options (Caido 0.59+): traffic Caido tunnels without intercepting or recording. ` +
			`Call with no arguments to read. outOfScope:true keeps history to in-scope requests only (scope comes from caido://scopes). ` +
			`allowlist/denylist replace the whole list; omitted fields are left as they are.`,
		Annotations: writeAnn(false, true, false),
	}, passthroughHandler(client))
}
