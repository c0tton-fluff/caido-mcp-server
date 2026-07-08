package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	gen "github.com/caido-community/sdk-go/graphql"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// SelectEnvironmentInput is the input for the tool
type SelectEnvironmentInput struct {
	ID string `json:"id" jsonschema:"required,Environment ID to select. Pass empty string to deselect."`
}

// SelectEnvironmentOutput is the output
type SelectEnvironmentOutput struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

func selectEnvironmentHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, SelectEnvironmentInput) (*mcp.CallToolResult, SelectEnvironmentOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input SelectEnvironmentInput,
	) (*mcp.CallToolResult, SelectEnvironmentOutput, error) {
		var idPtr *string
		if input.ID != "" {
			idPtr = &input.ID
		}

		resp, err := client.Environments.Select(ctx, idPtr)
		if err != nil {
			return nil, SelectEnvironmentOutput{}, err
		}

		payload := resp.GetSelectEnvironment()

		if errPtr := payload.GetError(); errPtr != nil {
			errIface := *errPtr
			typename := ""
			if t := errIface.GetTypename(); t != nil {
				typename = *t
			}
			// Check for OtherUserError with code.
			if other, ok := errIface.(*gen.SelectEnvironmentSelectEnvironmentSelectEnvironmentPayloadErrorOtherUserError); ok {
				return nil, SelectEnvironmentOutput{}, fmt.Errorf(
					"select environment failed: %s: %s",
					typename, other.GetCode(),
				)
			}
			return nil, SelectEnvironmentOutput{}, fmt.Errorf(
				"select environment failed: %s", typename,
			)
		}

		env := payload.GetEnvironment()
		if env == nil {
			return nil, SelectEnvironmentOutput{}, nil
		}

		return nil, SelectEnvironmentOutput{
			ID:   env.Id,
			Name: env.Name,
		}, nil
	}
}

// RegisterSelectEnvironmentTool registers the tool
func RegisterSelectEnvironmentTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_select_environment",
		Description: `Select active environment. Variables from selected env are used in replay placeholders. Pass empty id to deselect. Note: the Global environment (usually id "1") is always active and cannot be selected via this API.`,
		Annotations: writeAnn(false, true, false),
	}, selectEnvironmentHandler(client))
}
