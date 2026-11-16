package tools

import (
	"context"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ListPluginsInput struct{}

// PluginBackendSummary is the backend plugin of a package, the part that
// exposes callable functions.
type PluginBackendSummary struct {
	ID      string  `json:"id"`
	Name    *string `json:"name,omitempty"`
	Running bool    `json:"running"`
	Error   *string `json:"error,omitempty"`
}

type PluginSummary struct {
	ID          string  `json:"id"`
	ManifestID  string  `json:"manifestId"`
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Version     string  `json:"version"`
	// Backend is absent for frontend-only packages.
	Backend *PluginBackendSummary `json:"backend,omitempty"`
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
		packages, err := listPluginPackages(ctx, client)
		if err != nil {
			return nil, ListPluginsOutput{}, err
		}

		output := ListPluginsOutput{
			Packages: make([]PluginSummary, 0, len(packages)),
		}
		for _, p := range packages {
			summary := PluginSummary{
				ID:          p.ID,
				ManifestID:  p.ManifestID,
				Name:        p.Name,
				Description: p.Description,
				Version:     p.Version,
			}
			if backend := p.backend(); backend != nil {
				summary.Backend = &PluginBackendSummary{
					ID:      backend.ID,
					Name:    backend.Name,
					Running: backend.State == nil || backend.State.Running,
				}
				if backend.State != nil {
					summary.Backend.Error = backend.State.Error
				}
			}
			output.Packages = append(output.Packages, summary)
		}

		return nil, output, nil
	}
}

func RegisterListPluginsTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:  "caido_list_plugins",
		Title: "List Plugins",
		Description: `List installed Caido plugins with version info, package ` +
			`manifestId and backend state (id, running, error). A plugin whose ` +
			`backend is not running cannot be called until Caido restarts; ` +
			`use caido_call_plugin for the ones that are.`,
		InputSchema: map[string]any{"type": "object"},
		Annotations: readOnlyAnn(),
	}, listPluginsHandler(client))
}
