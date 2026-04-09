package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	gen "github.com/caido-community/sdk-go/graphql"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// CreateTamperRuleInput is the input for the create_tamper_rule tool
type CreateTamperRuleInput struct {
	CollectionID string   `json:"collection_id" jsonschema:"required,ID of the tamper rule collection"`
	Name         string   `json:"name" jsonschema:"required,Name for the new rule"`
	Condition    *string  `json:"condition,omitempty" jsonschema:"HTTPQL filter condition"`
	Sources      []string `json:"sources,omitempty" jsonschema:"Traffic sources: INTERCEPT REPLAY AUTOMATE IMPORT PLUGIN WORKFLOW SAMPLE"`
}

// CreateTamperRuleOutput is the output of the create_tamper_rule tool
type CreateTamperRuleOutput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// createTamperRuleHandler creates the handler function
func createTamperRuleHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, CreateTamperRuleInput) (*mcp.CallToolResult, CreateTamperRuleOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input CreateTamperRuleInput,
	) (*mcp.CallToolResult, CreateTamperRuleOutput, error) {
		if len(input.Name) > 200 {
			return nil, CreateTamperRuleOutput{}, fmt.Errorf(
				"name exceeds max length of 200",
			)
		}
		if input.Condition != nil && len(*input.Condition) > 10000 {
			return nil, CreateTamperRuleOutput{}, fmt.Errorf(
				"condition exceeds max length of 10000",
			)
		}

		sources := make([]gen.Source, 0, len(input.Sources))
		for _, s := range input.Sources {
			sources = append(sources, gen.Source(s))
		}

		gqlInput := &gen.CreateTamperRuleInput{
			CollectionId: input.CollectionID,
			Name:         input.Name,
			Condition:    input.Condition,
			Sources:      sources,
		}

		resp, err := client.Tamper.CreateRule(ctx, gqlInput)
		if err != nil {
			return nil, CreateTamperRuleOutput{}, err
		}

		payload := resp.CreateTamperRule
		if payload.Error != nil {
			errType := "unknown"
			if tn := (*payload.Error).GetTypename(); tn != nil {
				errType = *tn
			}
			return nil, CreateTamperRuleOutput{}, fmt.Errorf(
				"create tamper rule failed: %s", errType,
			)
		}

		if payload.Rule == nil {
			return nil, CreateTamperRuleOutput{}, fmt.Errorf(
				"create tamper rule returned no rule",
			)
		}

		return nil, CreateTamperRuleOutput{
			ID:   payload.Rule.Id,
			Name: payload.Rule.Name,
		}, nil
	}
}

// RegisterCreateTamperRuleTool registers the tool
func RegisterCreateTamperRuleTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "caido_create_tamper_rule",
		Description: `Create a Match & Replace (tamper) rule. ` +
			`Params: collection_id (required), name (required), ` +
			`condition (HTTPQL filter), sources (traffic sources).`,
	}, createTamperRuleHandler(client))
}
