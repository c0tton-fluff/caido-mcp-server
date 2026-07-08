package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	gen "github.com/caido-community/sdk-go/graphql"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type DeleteEnvironmentInput struct {
	ID string `json:"id" jsonschema:"required,Environment ID to delete"`
}

type DeleteEnvironmentOutput struct {
	Success bool `json:"success"`
}

func deleteEnvironmentHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, DeleteEnvironmentInput) (*mcp.CallToolResult, DeleteEnvironmentOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input DeleteEnvironmentInput,
	) (*mcp.CallToolResult, DeleteEnvironmentOutput, error) {
		if input.ID == "" {
			return nil, DeleteEnvironmentOutput{}, fmt.Errorf("id is required")
		}

		resp, err := client.Environments.Delete(ctx, input.ID)
		if err != nil {
			return nil, DeleteEnvironmentOutput{}, err
		}

		payload := resp.GetDeleteEnvironment()
		if errPtr := payload.GetError(); errPtr != nil {
			errIface := *errPtr
			typename := "unknown"
			if t := errIface.GetTypename(); t != nil {
				typename = *t
			}
			if other, ok := errIface.(*gen.DeleteEnvironmentDeleteEnvironmentDeleteEnvironmentPayloadErrorOtherUserError); ok {
				return nil, DeleteEnvironmentOutput{}, fmt.Errorf(
					"delete environment failed: %s: %s",
					typename, other.GetCode(),
				)
			}
			return nil, DeleteEnvironmentOutput{}, fmt.Errorf(
				"delete environment failed: %s", typename,
			)
		}

		return nil, DeleteEnvironmentOutput{
			Success: true,
		}, nil
	}
}

func RegisterDeleteEnvironmentTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_delete_environment",
		Description: `Delete an environment by ID. Note: the Global environment cannot be deleted.`,
		Annotations: writeAnn(true, true, false),
	}, deleteEnvironmentHandler(client))
}
