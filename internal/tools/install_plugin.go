package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	gen "github.com/caido-community/sdk-go/graphql"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type InstallPluginInput struct {
	URL        string `json:"url,omitempty" jsonschema:"URL of the plugin package to install"`
	ManifestID string `json:"manifestId,omitempty" jsonschema:"Store manifest ID of the plugin package to install"`
	Force      *bool  `json:"force,omitempty" jsonschema:"Force install even if already present"`
}

type InstallPluginOutput struct {
	ID   string  `json:"id"`
	Name *string `json:"name,omitempty"`
}

func installPluginHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, InstallPluginInput) (*mcp.CallToolResult, InstallPluginOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input InstallPluginInput,
	) (*mcp.CallToolResult, InstallPluginOutput, error) {
		if input.URL == "" && input.ManifestID == "" {
			return nil, InstallPluginOutput{}, fmt.Errorf("either url or manifestId is required")
		}

		source := gen.PluginPackageSource{}
		if input.URL != "" {
			source.Url = &input.URL
		}
		if input.ManifestID != "" {
			source.ManifestId = &input.ManifestID
		}

		resp, err := client.Plugins.InstallPackage(ctx, &gen.InstallPluginPackageInput{
			Source: source,
			Force:  input.Force,
		})
		if err != nil {
			return nil, InstallPluginOutput{}, err
		}

		payload := resp.InstallPluginPackage
		if payload.Error != nil {
			errType := "unknown"
			if tn := (*payload.Error).GetTypename(); tn != nil {
				errType = *tn
			}
			return nil, InstallPluginOutput{}, fmt.Errorf("install plugin failed: %s", errType)
		}
		if payload.Package == nil {
			return nil, InstallPluginOutput{}, fmt.Errorf("install plugin returned no package")
		}

		return nil, InstallPluginOutput{
			ID:   payload.Package.Id,
			Name: payload.Package.Name,
		}, nil
	}
}

func RegisterInstallPluginTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "caido_install_plugin",
		Description: `Install a Caido plugin package from a url or a store ` +
			`manifestId. Set force=true to reinstall.`,
	}, installPluginHandler(client))
}
