package tools

import (
	"context"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ListFiltersInput is the input for the list_filters tool
type ListFiltersInput struct{}

// FilterPresetSummary is a summary of a filter preset
type FilterPresetSummary struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Alias  string `json:"alias"`
	Clause string `json:"clause"`
}

// ListFiltersOutput is the output of the list_filters tool
type ListFiltersOutput struct {
	Filters []FilterPresetSummary `json:"filters"`
}

// listFiltersHandler creates the handler function
func listFiltersHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, ListFiltersInput) (*mcp.CallToolResult, ListFiltersOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input ListFiltersInput,
	) (*mcp.CallToolResult, ListFiltersOutput, error) {
		resp, err := client.Filters.List(ctx)
		if err != nil {
			return nil, ListFiltersOutput{}, err
		}

		output := ListFiltersOutput{
			Filters: make(
				[]FilterPresetSummary, 0,
				len(resp.FilterPresets),
			),
		}

		for _, f := range resp.FilterPresets {
			output.Filters = append(
				output.Filters, FilterPresetSummary{
					ID:     f.Id,
					Name:   f.Name,
					Alias:  f.Alias,
					Clause: f.Clause,
				},
			)
		}

		return nil, output, nil
	}
}

// RegisterListFiltersTool registers the tool
func RegisterListFiltersTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "caido_list_filters",
		Description: `List saved HTTPQL filter presets. ` +
			`Returns id/name/alias/clause.`,
	}, listFiltersHandler(client))
}
