package tools

import (
	"context"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ListFindingReportersInput struct{}

type ListFindingReportersOutput struct {
	Reporters []string `json:"reporters"`
}

func listFindingReportersHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, ListFindingReportersInput) (*mcp.CallToolResult, ListFindingReportersOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input ListFindingReportersInput,
	) (*mcp.CallToolResult, ListFindingReportersOutput, error) {
		resp, err := client.Findings.ListReporters(ctx)
		if err != nil {
			return nil, ListFindingReportersOutput{}, err
		}

		return nil, ListFindingReportersOutput{
			Reporters: resp.FindingReporters,
		}, nil
	}
}

func RegisterListFindingReportersTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "caido_list_finding_reporters",
		Description: `List all finding reporter names (the plugins/sources that have ` +
			`created findings). Useful as filter values for caido_list_findings.`,
		InputSchema: map[string]any{"type": "object"},
	}, listFindingReportersHandler(client))
}
