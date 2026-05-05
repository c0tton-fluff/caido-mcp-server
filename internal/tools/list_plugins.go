package tools

import (
	"context"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ListPluginsInput struct{}

type PluginSummary struct {
	ID          string  `json:"id"`
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Version     string  `json:"version"`
}

type ListPluginsOutput struct {
	Packages []PluginSummary `json:"packages"`
}

func listPluginsHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, ListPluginsInput) (*mcp.CallToolResult, ListPluginsOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input ListPluginsInput,
	) (*mcp.CallToolResult, ListPluginsOutput, error) {
		resp, err := client.Plugins.ListPackages(ctx)
		if err != nil {
			return nil, ListPluginsOutput{}, err
		}

		output := ListPluginsOutput{
			Packages: make([]PluginSummary, 0, len(resp.PluginPackages)),
		}
		for _, p := range resp.PluginPackages {
			output.Packages = append(output.Packages, PluginSummary{
				ID:          p.Id,
				Name:        p.Name,
				Description: p.Description,
				Version:     p.Version,
			})
		}

		return nil, output, nil
	}
}

func RegisterListPluginsTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_list_plugins",
		Description: `List installed Caido plugins with version info.`,
		InputSchema: map[string]any{"type": "object"},
	}, listPluginsHandler(client))
}
