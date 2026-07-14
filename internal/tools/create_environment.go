package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	gen "github.com/caido-community/sdk-go/graphql"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type CreateEnvironmentInput struct {
	Name string `json:"name" jsonschema:"required,Name of the environment"`
}

type CreateEnvironmentOutput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func createEnvironmentHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, CreateEnvironmentInput) (*mcp.CallToolResult, CreateEnvironmentOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input CreateEnvironmentInput,
	) (*mcp.CallToolResult, CreateEnvironmentOutput, error) {
		if input.Name == "" {
			return nil, CreateEnvironmentOutput{}, fmt.Errorf("name is required")
		}
		if len(input.Name) > 200 {
			return nil, CreateEnvironmentOutput{}, fmt.Errorf(
				"name exceeds max length of 200",
			)
		}

		// The GraphQL schema requires a non-null variables list
		// ([EnvironmentVariableInput!]!). A nil slice marshals to JSON null and
		// the server rejects it, so send an empty (non-nil) slice.
		resp, err := client.Environments.Create(ctx, &gen.CreateEnvironmentInput{
			Name:      input.Name,
			Variables: []gen.EnvironmentVariableInput{},
		})
		if err != nil {
			return nil, CreateEnvironmentOutput{}, err
		}

		payload := resp.CreateEnvironment
		if payload.Error != nil {
			errType := "unknown"
			if tn := (*payload.Error).GetTypename(); tn != nil {
				errType = *tn
			}
			return nil, CreateEnvironmentOutput{}, fmt.Errorf(
				"create environment failed: %s", errType,
			)
		}
		if payload.Environment == nil {
			return nil, CreateEnvironmentOutput{}, fmt.Errorf(
				"create environment returned no environment",
			)
		}

		return nil, CreateEnvironmentOutput{
			ID:   payload.Environment.Id,
			Name: payload.Environment.Name,
		}, nil
	}
}

func RegisterCreateEnvironmentTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_create_environment",
		Description: `Create a new environment. Environments store variables (tokens, keys, etc) that can be used in replay placeholders.`,
		Annotations: writeAnn(false, false, false),
	}, createEnvironmentHandler(client))
}
