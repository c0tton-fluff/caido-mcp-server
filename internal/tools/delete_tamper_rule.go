package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// DeleteTamperRuleInput is the input for the delete_tamper_rule tool
type DeleteTamperRuleInput struct {
	ID string `json:"id" jsonschema:"required,Tamper rule ID to delete"`
}

// DeleteTamperRuleOutput is the output of the delete_tamper_rule tool
type DeleteTamperRuleOutput struct {
	DeletedID string `json:"deleted_id"`
}

// deleteTamperRuleHandler creates the handler function
func deleteTamperRuleHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, DeleteTamperRuleInput) (*mcp.CallToolResult, DeleteTamperRuleOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input DeleteTamperRuleInput,
	) (*mcp.CallToolResult, DeleteTamperRuleOutput, error) {
		resp, err := client.Tamper.DeleteRule(ctx, input.ID)
		if err != nil {
			return nil, DeleteTamperRuleOutput{}, err
		}

		payload := resp.DeleteTamperRule
		if payload.DeletedId == nil {
			return nil, DeleteTamperRuleOutput{}, fmt.Errorf(
				"delete tamper rule returned no ID",
			)
		}

		return nil, DeleteTamperRuleOutput{
			DeletedID: *payload.DeletedId,
		}, nil
	}
}

// RegisterDeleteTamperRuleTool registers the tool
func RegisterDeleteTamperRuleTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "caido_delete_tamper_rule",
		Description: `Delete a Match & Replace (tamper) rule ` +
			`by ID. Params: id (required).`,
		Annotations: writeAnn(true, true, false),
	}, deleteTamperRuleHandler(client))
}
