package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	gen "github.com/caido-community/sdk-go/graphql"
	"github.com/caido-community/sdk-go/graphql/scalars"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type CreateFilterInput struct {
	Name  string `json:"name" jsonschema:"required,Name of the filter preset"`
	Query string `json:"query" jsonschema:"required,HTTPQL query string"`
	Alias string `json:"alias,omitempty" jsonschema:"Optional alias for the filter"`
}

type CreateFilterOutput struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Alias string `json:"alias"`
}

func createFilterHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, CreateFilterInput) (*mcp.CallToolResult, CreateFilterOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input CreateFilterInput,
	) (*mcp.CallToolResult, CreateFilterOutput, error) {
		resp, err := client.Filters.Create(ctx, &gen.CreateFilterPresetInput{
			Name:  input.Name,
			Alias: input.Alias,
			Clause: gen.QueryInput{
				HTTPQL: &scalars.HTTPQLInput{
					Code: input.Query,
				},
			},
		})
		if err != nil {
			return nil, CreateFilterOutput{}, err
		}

		payload := resp.CreateFilterPreset
		if payload.Error != nil {
			errType := "unknown"
			if payload.Error != nil {
				if tn := (*payload.Error).GetTypename(); tn != nil {
					errType = *tn
				}
			}
			return nil, CreateFilterOutput{}, fmt.Errorf(
				"failed to create filter: %s", errType,
			)
		}

		filter := payload.Filter
		if filter == nil {
			return nil, CreateFilterOutput{}, fmt.Errorf(
				"no filter preset returned",
			)
		}

		output := CreateFilterOutput{
			ID:    filter.Id,
			Name:  filter.Name,
			Alias: filter.Alias,
		}

		return nil, output, nil
	}
}

func RegisterCreateFilterTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "caido_create_filter",
		Description: `Create a new HTTPQL filter preset. ` +
			`Returns id/name/alias of the created filter.`,
	}, createFilterHandler(client))
}
