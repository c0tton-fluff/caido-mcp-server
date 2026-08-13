package tools

import (
	"context"
	"strings"

	caido "github.com/caido-community/sdk-go"
	gen "github.com/caido-community/sdk-go/graphql"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ListTamperRulesInput is the input for the list_tamper_rules tool
type ListTamperRulesInput struct{}

// TamperRuleSummary is a summary of a tamper rule
type TamperRuleSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Section is the part of the request or response the rule acts on,
	// using the same names the create/update/test tools accept.
	//
	// The operation body (mode, matcher, replacer) is deliberately not
	// surfaced: reaching it through the generated union types costs ~30
	// type assertions, and caido_test_tamper_rule answers "what does this
	// rule actually do" with ground truth instead of a static summary.
	Section   string   `json:"section,omitempty"`
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
					Section:   tamperSectionFromTypename(r.Section),
					Enabled:   enabled,
					Condition: tamperRuleConditionToString(r.Condition),
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

// tamperSectionFromTypename converts a GraphQL section __typename such as
// "TamperSectionRequestHeader" into the tool-facing section name
// "requestHeader", so a listed rule can be fed straight back into the
// create/update/test tools.
//
// Returns "" when the rule has no section, or when the section is a valid
// union member the tools do not expose (the two streamWsMessage sections).
// A __typename outside the union never reaches here: genqlient's generated
// unmarshaller fails the whole response first, so the only cases to handle
// are a nil section and a known-but-unexposed one.
func tamperSectionFromTypename(
	section gen.ListTamperRuleCollectionsTamperRuleCollectionsTamperRuleCollectionRulesTamperRuleSectionTamperSection,
) string {
	if section == nil {
		return ""
	}
	typename := section.GetTypename()
	if typename == nil {
		return ""
	}
	name := strings.TrimPrefix(*typename, "TamperSection")
	if name == "" {
		return ""
	}
	candidate := strings.ToLower(name[:1]) + name[1:]
	if _, ok := tamperSections[candidate]; !ok {
		return ""
	}
	return candidate
}

func tamperRuleConditionToString(
	cond *gen.ListTamperRuleCollectionsTamperRuleCollectionsTamperRuleCollectionRulesTamperRuleConditionQuery,
) *string {
	if cond == nil {
		return nil
	}
	var s string
	switch v := (*cond).(type) {
	case *gen.ListTamperRuleCollectionsTamperRuleCollectionsTamperRuleCollectionRulesTamperRuleConditionHTTPQL:
		s = v.Code
	case *gen.ListTamperRuleCollectionsTamperRuleCollectionsTamperRuleCollectionRulesTamperRuleConditionStreamQL:
		s = v.Code
	default:
		return nil
	}
	return &s
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
			`(id/name/section/enabled/condition/sources). ` +
			`Use caido_test_tamper_rule to see what a rule does to a ` +
			`given request.`,
		Annotations: readOnlyAnn(),
	}, listTamperRulesHandler(client))
}
