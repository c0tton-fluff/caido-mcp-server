package tools

import (
	"context"

	caido "github.com/caido-community/sdk-go"
	gen "github.com/caido-community/sdk-go/graphql"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type WhoamiInput struct{}

type WhoamiOutput struct {
	Kind  string `json:"kind"`
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
}

func whoamiHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, WhoamiInput) (*mcp.CallToolResult, WhoamiOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input WhoamiInput,
	) (*mcp.CallToolResult, WhoamiOutput, error) {
		resp, err := client.Users.GetViewer(ctx)
		if err != nil {
			return nil, WhoamiOutput{}, err
		}

		out := WhoamiOutput{}
		switch u := resp.Viewer.(type) {
		case *gen.GetViewerViewerCloudUser:
			out.Kind = "cloud"
			out.ID = u.Id
			out.Name = u.Profile.Identity.Name
			out.Email = u.Profile.Identity.Email
		case *gen.GetViewerViewerGuestUser:
			out.Kind = "guest"
		case *gen.GetViewerViewerScriptUser:
			out.Kind = "script"
		default:
			out.Kind = "unknown"
		}
		return nil, out, nil
	}
}

func RegisterWhoamiTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "caido_whoami",
		Description: `Get the currently authenticated Caido user (kind: cloud/guest/script, ` +
			`plus name and email for cloud users).`,
		InputSchema: map[string]any{"type": "object"},
	}, whoamiHandler(client))
}
