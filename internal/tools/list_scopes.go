package tools

import (
	"context"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ListScopesInput is the input for the list_scopes tool
type ListScopesInput struct{}

// ScopeSummary is a summary of a scope
type ScopeSummary struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Allowlist []string `json:"allowlist"`
	Denylist  []string `json:"denylist"`
	Indexed   bool     `json:"indexed"`
}

// ListScopesOutput is the output of the list_scopes tool
type ListScopesOutput struct {
	Scopes []ScopeSummary `json:"scopes"`
}

// listScopesHandler creates the handler function
func listScopesHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, ListScopesInput) (*mcp.CallToolResult, ListScopesOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input ListScopesInput,
	) (*mcp.CallToolResult, ListScopesOutput, error) {
		resp, err := client.Scopes.List(ctx)
		if err != nil {
			return nil, ListScopesOutput{}, err
		}

		output := ListScopesOutput{
			Scopes: make([]ScopeSummary, 0, len(resp.Scopes)),
		}

		for _, s := range resp.Scopes {
			output.Scopes = append(output.Scopes, ScopeSummary{
				ID:        s.Id,
				Name:      s.Name,
				Allowlist: s.Allowlist,
				Denylist:  s.Denylist,
				Indexed:   s.Indexed,
			})
		}

		return nil, output, nil
	}
}

// RegisterListScopesTool registers the tool with the MCP server
func RegisterListScopesTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_list_scopes",
		Description: `List scopes. Returns name/allowlist/denylist.`,
		InputSchema: map[string]any{"type": "object"},
	}, listScopesHandler(client))
}
