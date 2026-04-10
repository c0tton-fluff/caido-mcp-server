package tools

import (
	"context"
	"fmt"

	gql "github.com/Khan/genqlient/graphql"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type deleteFindingsByIDsVars struct {
	Input *deleteFindingsByIDsInput `json:"input"`
}

type deleteFindingsByIDsInput struct {
	Ids []string `json:"ids"`
}

type deleteFindingsByReporterVars struct {
	Input *deleteFindingsByReporterInput `json:"input"`
}

type deleteFindingsByReporterInput struct {
	Reporter string `json:"reporter"`
}

type deleteFindingsResp struct {
	DeleteFindings struct {
		DeletedIds []string `json:"deletedIds"`
	} `json:"deleteFindings"`
}

const deleteFindingsMutation = `
mutation DeleteFindings ($input: DeleteFindingsInput!) {
	deleteFindings(input: $input) {
		deletedIds
	}
}`

func deleteFindingsRaw(
	ctx context.Context,
	gqlClient gql.Client,
	ids []string,
	reporter string,
) (*deleteFindingsResp, error) {
	var vars any
	if len(ids) > 0 {
		vars = &deleteFindingsByIDsVars{
			Input: &deleteFindingsByIDsInput{Ids: ids},
		}
	} else {
		vars = &deleteFindingsByReporterVars{
			Input: &deleteFindingsByReporterInput{Reporter: reporter},
		}
	}

	req := &gql.Request{
		OpName:    "DeleteFindings",
		Query:     deleteFindingsMutation,
		Variables: vars,
	}
	data := &deleteFindingsResp{}
	resp := &gql.Response{Data: data}
	if err := gqlClient.MakeRequest(ctx, req, resp); err != nil {
		return nil, err
	}
	return data, nil
}

// DeleteFindingsInput is the input for the tool
type DeleteFindingsInput struct {
	IDs      []string `json:"ids,omitempty" jsonschema:"List of finding IDs to delete"`
	Reporter string   `json:"reporter,omitempty" jsonschema:"Delete all findings by this reporter name"`
}

// DeleteFindingsOutput is the output
type DeleteFindingsOutput struct {
	DeletedIDs []string `json:"deletedIds"`
}

func deleteFindingsHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, DeleteFindingsInput) (*mcp.CallToolResult, DeleteFindingsOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input DeleteFindingsInput,
	) (*mcp.CallToolResult, DeleteFindingsOutput, error) {
		if len(input.IDs) == 0 && input.Reporter == "" {
			return nil, DeleteFindingsOutput{}, fmt.Errorf(
				"provide either ids or reporter",
			)
		}

		resp, err := deleteFindingsRaw(
			ctx, client.GraphQL, input.IDs, input.Reporter,
		)
		if err != nil {
			return nil, DeleteFindingsOutput{}, err
		}

		return nil, DeleteFindingsOutput{
			DeletedIDs: resp.DeleteFindings.DeletedIds,
		}, nil
	}
}

// RegisterDeleteFindingsTool registers the tool
func RegisterDeleteFindingsTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_delete_findings",
		Description: `Delete findings by IDs or by reporter name. Params: ids (list) or reporter (string).`,
	}, deleteFindingsHandler(client))
}
