package resources

import (
	"context"
	"fmt"
	"strings"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerProjectResource(server *mcp.Server, client *caido.Client) {
	server.AddResource(
		&mcp.Resource{
			URI:         "caido://project",
			Name:        "caido-project",
			Description: "Current Caido project, instance version, and connection status",
			MIMEType:    "text/plain",
		},
		projectHandler(client),
	)
}

func projectHandler(client *caido.Client) mcp.ResourceHandler {
	return func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		runtimeResp, err := client.Instance.GetRuntime(ctx)
		if err != nil {
			return nil, fmt.Errorf("get instance runtime: %w", err)
		}
		rt := runtimeResp.Runtime

		var b strings.Builder
		b.WriteString("# Caido Project\n\n")
		b.WriteString("Connection: ok\n")
		fmt.Fprintf(&b, "Instance version: %s\n", rt.Version)
		fmt.Fprintf(&b, "Platform: %s\n", rt.Platform)

		// A nil CurrentProject (or a query error) means no project is currently
		// selected in Caido. That is a normal, expected state, not a failure -
		// still surface instance/connection info instead of failing hard.
		projResp, projErr := client.Projects.GetCurrent(ctx)
		if projErr != nil || projResp.CurrentProject == nil {
			b.WriteString("Project: no project selected\n")
			return &mcp.ReadResourceResult{
				Contents: []*mcp.ResourceContents{{
					URI:  req.Params.URI,
					Text: b.String(),
				}},
			}, nil
		}

		p := projResp.CurrentProject.Project
		fmt.Fprintf(&b, "Project: %s\n", p.Name)
		fmt.Fprintf(&b, "Project ID: %s\n", p.Id)
		fmt.Fprintf(&b, "Project status: %s\n", string(p.Status))

		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{
				URI:  req.Params.URI,
				Text: b.String(),
			}},
		}, nil
	}
}
