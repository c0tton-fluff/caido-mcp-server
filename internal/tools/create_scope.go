package tools

import (
	"context"
	"fmt"

	"github.com/c0tton-fluff/caido-mcp-server/internal/caido"
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
func createScopeHandler(client *caido.Client) func(context.Context, *mcp.CallToolRequest, CreateScopeInput) (*mcp.CallToolResult, CreateScopeOutput, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input CreateScopeInput) (*mcp.CallToolResult, CreateScopeOutput, error) {
		if input.Name == "" {
			return nil, CreateScopeOutput{}, fmt.Errorf("name is required")
		}
		if len(input.Allowlist) == 0 {
			return nil, CreateScopeOutput{}, fmt.Errorf("allowlist is required")
		}

		scopeInput := caido.CreateScopeInput{
			Name:      input.Name,
			Allowlist: input.Allowlist,
			Denylist:  input.Denylist,
		}

		scope, err := client.CreateScope(ctx, scopeInput)
		if err != nil {
			return nil, CreateScopeOutput{}, err
		}

		return nil, CreateScopeOutput{
			ID:   scope.ID,
			Name: scope.Name,
		}, nil
	}
}

// RegisterCreateScopeTool registers the tool with the MCP server
func RegisterCreateScopeTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_create_scope",
		Description: `Create scope. Params: name, allowlist (e.g. "*://example.com/*"), denylist.`,
	}, createScopeHandler(client))
}
