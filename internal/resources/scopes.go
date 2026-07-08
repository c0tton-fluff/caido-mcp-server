package resources

import (
	"context"
	"fmt"
	"strings"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerScopesResource(server *mcp.Server, client *caido.Client) {
	server.AddResource(
		&mcp.Resource{
			URI:         "caido://scopes",
			Name:        "caido-scopes",
			Description: "Scopes with their allow/deny rules",
			MIMEType:    "text/plain",
		},
		scopesHandler(client),
	)
}

func scopesHandler(client *caido.Client) mcp.ResourceHandler {
	return func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		resp, err := client.Scopes.List(ctx)
		if err != nil {
			return nil, fmt.Errorf("list scopes: %w", err)
		}

		var b strings.Builder
		b.WriteString("# Scopes\n\n")

		for _, s := range resp.Scopes {
			fmt.Fprintf(&b, "## %s\n", s.Name)
			fmt.Fprintf(&b, "- ID: %s\n", s.Id)
			fmt.Fprintf(&b, "- Indexed: %t\n", s.Indexed)
			if len(s.Allowlist) > 0 {
				fmt.Fprintf(&b, "- Allowlist: %s\n", strings.Join(s.Allowlist, ", "))
			} else {
				b.WriteString("- Allowlist: (none)\n")
			}
			if len(s.Denylist) > 0 {
				fmt.Fprintf(&b, "- Denylist: %s\n", strings.Join(s.Denylist, ", "))
			} else {
				b.WriteString("- Denylist: (none)\n")
			}
			b.WriteString("\n")
		}

		if len(resp.Scopes) == 0 {
			b.WriteString("(no scopes configured)\n")
		}

		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{
				URI:  req.Params.URI,
				Text: b.String(),
			}},
		}, nil
	}
}
