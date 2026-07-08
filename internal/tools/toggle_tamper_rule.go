package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToggleTamperRuleInput is the input for the toggle_tamper_rule tool
type ToggleTamperRuleInput struct {
	ID      string `json:"id" jsonschema:"required,Tamper rule ID"`
	Enabled bool   `json:"enabled" jsonschema:"required,Enable (true) or disable (false) the rule"`
}

// ToggleTamperRuleOutput is the output of the toggle_tamper_rule tool
type ToggleTamperRuleOutput struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// toggleTamperRuleHandler creates the handler function
func toggleTamperRuleHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, ToggleTamperRuleInput) (*mcp.CallToolResult, ToggleTamperRuleOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input ToggleTamperRuleInput,
	) (*mcp.CallToolResult, ToggleTamperRuleOutput, error) {
		resp, err := client.Tamper.ToggleRule(
			ctx, input.ID, input.Enabled,
		)
		if err != nil {
			return nil, ToggleTamperRuleOutput{}, err
		}

		payload := resp.ToggleTamperRule
		if payload.Error != nil {
			errType := "unknown"
			if tn := (*payload.Error).GetTypename(); tn != nil {
				errType = *tn
			}
			return nil, ToggleTamperRuleOutput{}, fmt.Errorf(
				"toggle tamper rule failed: %s", errType,
			)
		}

		if payload.Rule == nil {
			return nil, ToggleTamperRuleOutput{}, fmt.Errorf(
				"toggle tamper rule returned no rule",
			)
		}

		enabled := payload.Rule.Enable != nil

		return nil, ToggleTamperRuleOutput{
			ID:      payload.Rule.Id,
			Name:    payload.Rule.Name,
			Enabled: enabled,
		}, nil
	}
}

// RegisterToggleTamperRuleTool registers the tool
func RegisterToggleTamperRuleTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "caido_toggle_tamper_rule",
		Description: `Enable or disable a Match & Replace ` +
			`(tamper) rule. Params: id (required), ` +
			`enabled (true/false).`,
		Annotations: writeAnn(false, false, false),
	}, toggleTamperRuleHandler(client))
}
