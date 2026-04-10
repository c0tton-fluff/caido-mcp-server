package tools

import (
	"context"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ListTamperRulesInput is the input for the list_tamper_rules tool
type ListTamperRulesInput struct{}

// TamperRuleSummary is a summary of a tamper rule
type TamperRuleSummary struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Enabled   bool     `json:"enabled"`
	Condition *string  `json:"condition,omitempty"`
	Sources   []string `json:"sources"`
}

// TamperCollectionSummary is a summary of a tamper collection
type TamperCollectionSummary struct {
	ID    string              `json:"id"`
	Name  string              `json:"name"`
	Rules []TamperRuleSummary `json:"rules"`
}

// ListTamperRulesOutput is the output of the list_tamper_rules tool
type ListTamperRulesOutput struct {
	Collections []TamperCollectionSummary `json:"collections"`
}

// listTamperRulesHandler creates the handler function
func listTamperRulesHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, ListTamperRulesInput) (*mcp.CallToolResult, ListTamperRulesOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input ListTamperRulesInput,
	) (*mcp.CallToolResult, ListTamperRulesOutput, error) {
		resp, err := client.Tamper.ListCollections(ctx)
		if err != nil {
			return nil, ListTamperRulesOutput{}, err
		}

		output := ListTamperRulesOutput{
			Collections: make(
				[]TamperCollectionSummary, 0,
				len(resp.TamperRuleCollections),
			),
		}

		for _, c := range resp.TamperRuleCollections {
			col := TamperCollectionSummary{
				ID:   c.Id,
				Name: c.Name,
				Rules: make(
					[]TamperRuleSummary, 0,
					len(c.Rules),
				),
			}

			for _, r := range c.Rules {
				enabled := r.Enable != nil
				sources := make([]string, 0, len(r.Sources))
				for _, s := range r.Sources {
					sources = append(sources, string(s))
				}

				col.Rules = append(col.Rules, TamperRuleSummary{
					ID:        r.Id,
					Name:      r.Name,
					Enabled:   enabled,
					Condition: r.Condition,
					Sources:   sources,
				})
			}

			output.Collections = append(
				output.Collections, col,
			)
		}

		return nil, output, nil
	}
}

// RegisterListTamperRulesTool registers the tool
func RegisterListTamperRulesTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "caido_list_tamper_rules",
		Description: `List Match & Replace (tamper) rule ` +
			`collections and their rules. Returns ` +
			`collection id/name with nested rules ` +
			`(id/name/enabled/condition/sources).`,
	}, listTamperRulesHandler(client))
}
