package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type TogglePluginInput struct {
	ID      string `json:"id" jsonschema:"required,Plugin ID"`
	Enabled bool   `json:"enabled" jsonschema:"required,Enable (true) or disable (false) the plugin"`
}

type TogglePluginOutput struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
}

func togglePluginHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, TogglePluginInput) (*mcp.CallToolResult, TogglePluginOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input TogglePluginInput,
	) (*mcp.CallToolResult, TogglePluginOutput, error) {
		resp, err := client.Plugins.Toggle(ctx, input.ID, input.Enabled)
		if err != nil {
			return nil, TogglePluginOutput{}, err
		}

		payload := resp.TogglePlugin
		if payload.Error != nil {
			errType := "unknown"
			if tn := (*payload.Error).GetTypename(); tn != nil {
				errType = *tn
			}
			return nil, TogglePluginOutput{}, fmt.Errorf("toggle plugin failed: %s", errType)
		}
		if payload.Plugin == nil {
			return nil, TogglePluginOutput{}, fmt.Errorf("toggle plugin returned no plugin")
		}

		plugin := *payload.Plugin
		return nil, TogglePluginOutput{
			ID:      plugin.GetId(),
			Enabled: plugin.GetEnabled(),
		}, nil
	}
}

func RegisterTogglePluginTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_toggle_plugin",
		Description: `Enable or disable an installed plugin by ID.`,
	}, togglePluginHandler(client))
}
