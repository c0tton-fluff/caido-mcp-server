package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type DeleteInterceptEntriesInput struct {
	Filter  string  `json:"filter,omitempty" jsonschema:"HTTPQL filter selecting entries to delete (omit to clear all)"`
	ScopeID *string `json:"scopeId,omitempty" jsonschema:"Restrict deletion to this scope ID"`
}

type DeleteInterceptEntriesOutput struct {
	Success bool `json:"success"`
}

func deleteInterceptEntriesHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, DeleteInterceptEntriesInput) (*mcp.CallToolResult, DeleteInterceptEntriesOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input DeleteInterceptEntriesInput,
	) (*mcp.CallToolResult, DeleteInterceptEntriesOutput, error) {
		if len(input.Filter) > 10000 {
			return nil, DeleteInterceptEntriesOutput{}, fmt.Errorf("filter exceeds max length of 10000")
		}

		var filter *string
		if input.Filter != "" {
			filter = &input.Filter
		}

		_, err := client.Intercept.DeleteEntries(ctx, filter, input.ScopeID)
		if err != nil {
			return nil, DeleteInterceptEntriesOutput{}, err
		}

		return nil, DeleteInterceptEntriesOutput{Success: true}, nil
	}
}

func RegisterDeleteInterceptEntriesTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "caido_delete_intercept_entries",
		Description: `Delete intercept queue entries matching an HTTPQL filter ` +
			`(omit filter to clear the entire queue). Runs as a background task.`,
	}, deleteInterceptEntriesHandler(client))
}
