package tools

import (
	"context"
	"fmt"

	gql "github.com/Khan/genqlient/graphql"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type exportFindingsByIDsVars struct {
	Input *exportFindingsByIDsInput `json:"input"`
}

type exportFindingsByIDsInput struct {
	Ids []string `json:"ids"`
}

type exportFindingsByFilterVars struct {
	Input *exportFindingsByFilterInput `json:"input"`
}

type exportFindingsByFilterInput struct {
	Filter *exportFindingsFilter `json:"filter"`
}

type exportFindingsFilter struct {
	Reporter string `json:"reporter"`
}

type exportFindingsPayload struct {
	Export *struct {
		Id string `json:"id"`
	} `json:"export"`
	Error *struct {
		Typename string `json:"__typename"`
	} `json:"error"`
}

type exportFindingsResp struct {
	ExportFindings *exportFindingsPayload `json:"exportFindings"`
}

const exportFindingsMutation = `
mutation ExportFindings ($input: ExportFindingsInput!) {
	exportFindings(input: $input) {
		export { id }
		error { __typename ... on OtherUserError { code } }
	}
}`

func exportFindingsRaw(
	ctx context.Context,
	gqlClient gql.Client,
	ids []string,
	reporter string,
) (*exportFindingsResp, error) {
	var vars any
	if len(ids) > 0 {
		vars = &exportFindingsByIDsVars{
			Input: &exportFindingsByIDsInput{Ids: ids},
		}
	} else {
		vars = &exportFindingsByFilterVars{
			Input: &exportFindingsByFilterInput{
				Filter: &exportFindingsFilter{Reporter: reporter},
			},
		}
	}

	req := &gql.Request{
		OpName:    "ExportFindings",
		Query:     exportFindingsMutation,
		Variables: vars,
	}
	data := &exportFindingsResp{}
	resp := &gql.Response{Data: data}
	if err := gqlClient.MakeRequest(ctx, req, resp); err != nil {
		return nil, err
	}
	return data, nil
}

// ExportFindingsInput is the input for the tool
type ExportFindingsInput struct {
	IDs      []string `json:"ids,omitempty" jsonschema:"List of finding IDs to export"`
	Reporter string   `json:"reporter,omitempty" jsonschema:"Export all findings by this reporter name"`
}

// ExportFindingsOutput is the output
type ExportFindingsOutput struct {
	ExportID string `json:"exportId"`
}

func exportFindingsHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, ExportFindingsInput) (*mcp.CallToolResult, ExportFindingsOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input ExportFindingsInput,
	) (*mcp.CallToolResult, ExportFindingsOutput, error) {
		if len(input.IDs) == 0 && input.Reporter == "" {
			return nil, ExportFindingsOutput{}, fmt.Errorf(
				"provide either ids or reporter",
			)
		}

		resp, err := exportFindingsRaw(
			ctx, client.GraphQL, input.IDs, input.Reporter,
		)
		if err != nil {
			return nil, ExportFindingsOutput{}, err
		}

		payload := resp.ExportFindings
		if payload == nil {
			return nil, ExportFindingsOutput{}, fmt.Errorf(
				"export findings returned no payload",
			)
		}
		if payload.Error != nil {
			return nil, ExportFindingsOutput{}, fmt.Errorf(
				"export findings failed: %s",
				payload.Error.Typename,
			)
		}
		if payload.Export == nil {
			return nil, ExportFindingsOutput{}, fmt.Errorf(
				"export findings returned no export",
			)
		}

		return nil, ExportFindingsOutput{
			ExportID: payload.Export.Id,
		}, nil
	}
}

// RegisterExportFindingsTool registers the tool
func RegisterExportFindingsTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_export_findings",
		Description: `Export findings. Filter by IDs or reporter name. Returns exportId for download.`,
	}, exportFindingsHandler(client))
}
