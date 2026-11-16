package tools

import (
	"context"
	"fmt"

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

// requireScope fails when no scope has the given ID. Caido ignores an
// unknown scopeId and answers with the unscoped listing (seen live on
// 0.59.1: same totalCount as no scope at all), which would let an agent
// holding a stale ID treat out-of-scope traffic as in scope.
func requireScope(ctx context.Context, client *caido.Client, id string) error {
	resp, err := client.Scopes.List(ctx)
	if err != nil {
		return fmt.Errorf("check scope %s: %w", id, err)
	}
	for _, s := range resp.Scopes {
		if s.Id == id {
			return nil
		}
	}
	return fmt.Errorf("scope %s not found; list scopes with caido_list_scopes", id)
}

// RegisterListScopesTool registers the tool with the MCP server
func RegisterListScopesTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_list_scopes",
		Title:       "List Scopes",
		Description: `List scopes. Returns name/allowlist/denylist.`,
		InputSchema: map[string]any{"type": "object"},
		Annotations: readOnlyAnn(),
	}, listScopesHandler(client))
}
