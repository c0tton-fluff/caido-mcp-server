package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type GetTamperRuleInput struct {
	ID string `json:"id" jsonschema:"required,Tamper rule ID"`
}

type GetTamperRuleOutput struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Enabled        bool     `json:"enabled"`
	Sources        []string `json:"sources"`
	CollectionID   string   `json:"collectionId"`
	CollectionName string   `json:"collectionName"`
}

func getTamperRuleHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, GetTamperRuleInput) (*mcp.CallToolResult, GetTamperRuleOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input GetTamperRuleInput,
	) (*mcp.CallToolResult, GetTamperRuleOutput, error) {
		resp, err := client.Tamper.GetRule(ctx, input.ID)
		if err != nil {
			return nil, GetTamperRuleOutput{}, err
		}

		rule := resp.TamperRule
		if rule == nil {
			return nil, GetTamperRuleOutput{}, fmt.Errorf("tamper rule not found")
		}

		sources := make([]string, 0, len(rule.Sources))
		for _, s := range rule.Sources {
			sources = append(sources, string(s))
		}

		return nil, GetTamperRuleOutput{
			ID:             rule.Id,
			Name:           rule.Name,
			Enabled:        rule.Enable != nil,
			Sources:        sources,
			CollectionID:   rule.Collection.Id,
			CollectionName: rule.Collection.Name,
		}, nil
	}
}

func RegisterGetTamperRuleTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "caido_get_tamper_rule",
		Description: `Get a Match & Replace (tamper) rule by ID: name, whether it's ` +
			`enabled, traffic sources, and its collection.`,
	}, getTamperRuleHandler(client))
}
