package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	gen "github.com/caido-community/sdk-go/graphql"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// CreateScopeInput is the input for the create_scope tool
type CreateScopeInput struct {
	Name      string   `json:"name" jsonschema:"required,Name of the scope"`
	Allowlist []string `json:"allowlist" jsonschema:"required,Patterns to include (e.g. *://example.com/*)"`
	Denylist  []string `json:"denylist,omitempty" jsonschema:"Patterns to exclude"`
}

// CreateScopeOutput is the output of the create_scope tool
type CreateScopeOutput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// createScopeHandler creates the handler function
func createScopeHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, CreateScopeInput) (*mcp.CallToolResult, CreateScopeOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input CreateScopeInput,
	) (*mcp.CallToolResult, CreateScopeOutput, error) {
		if input.Name == "" {
			return nil, CreateScopeOutput{}, fmt.Errorf("name is required")
		}
		if len(input.Name) > 200 {
			return nil, CreateScopeOutput{}, fmt.Errorf(
				"name exceeds max length of 200",
			)
		}
		if len(input.Allowlist) == 0 {
			return nil, CreateScopeOutput{}, fmt.Errorf("allowlist is required")
		}
		if len(input.Allowlist) > 100 {
			return nil, CreateScopeOutput{}, fmt.Errorf(
				"allowlist exceeds max of 100 entries",
			)
		}

		denylist := input.Denylist
		if denylist == nil {
			denylist = []string{}
		}

		resp, err := client.Scopes.Create(ctx, &gen.CreateScopeInput{
			Name:      input.Name,
			Allowlist: input.Allowlist,
			Denylist:  denylist,
		})
		if err != nil {
			return nil, CreateScopeOutput{}, err
		}

		payload := resp.CreateScope
		if payload.Error != nil {
			errType := "unknown"
			if payload.Error != nil {
				if tn := (*payload.Error).GetTypename(); tn != nil {
					errType = *tn
				}
			}
			return nil, CreateScopeOutput{}, fmt.Errorf(
				"create scope failed: %s", errType,
			)
		}
		if payload.Scope == nil {
			return nil, CreateScopeOutput{}, fmt.Errorf(
				"create scope returned no scope",
			)
		}

		return nil, CreateScopeOutput{
			ID:   payload.Scope.Id,
			Name: payload.Scope.Name,
		}, nil
	}
}

// RegisterCreateScopeTool registers the tool with the MCP server
func RegisterCreateScopeTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "caido_create_scope",
		Description: `Create scope. Params: name, allowlist, denylist. ` +
			`Values are hostnames, not URLs. ` +
			`Examples: "example.com", "*.example.com". ` +
			`Do NOT include scheme (https://) or paths.`,
	}, createScopeHandler(client))
}
