package tools

import (
	"context"
	"fmt"

	gql "github.com/Khan/genqlient/graphql"
	caido "github.com/caido-community/sdk-go"
	gen "github.com/caido-community/sdk-go/graphql"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// createTamperRuleVars is the raw GraphQL variable wrapper.
// We use map[string]any for section because TamperSectionInput
// and its nested operation/matcher/replacer types are all GraphQL
// oneofs. genqlient v0.8.1 (the latest release as of 2026-06) drops
// omitempty from nullable pointer fields, causing unset oneof variants
// to serialize as null. Maps only include keys we set, avoiding oneof
// violations at every nesting level. This is an upstream genqlient
// limitation: keep the workaround until a release fixes omitempty on
// nullable pointer fields in GraphQL oneofs (no such release exists yet;
// v0.8.1 is current, no v0.9.x). Tracked in .claude/TODO.md.
//
// The section value itself is built by buildTamperSection in
// tamper_section.go, which owns the section/operation shape table.
type createTamperRuleVars struct {
	Input createTamperRuleGQLInput `json:"input"`
}

type createTamperRuleGQLInput struct {
	CollectionId string         `json:"collectionId"`
	Name         string         `json:"name"`
	Section      map[string]any `json:"section"`
	Condition    map[string]any `json:"condition,omitempty"`
	Sources      []gen.Source   `json:"sources"`
}

type createTamperRuleResp struct {
	CreateTamperRule struct {
		Rule *struct {
			Id   string `json:"id"`
			Name string `json:"name"`
		} `json:"rule"`
		Error *struct {
			Typename string `json:"__typename"`
		} `json:"error"`
	} `json:"createTamperRule"`
}

const createTamperRuleMutation = `
mutation CreateTamperRule($input: CreateTamperRuleInput!) {
	createTamperRule(input: $input) {
		error { __typename }
		rule { id name }
	}
}`

// CreateTamperRuleInput is the input for the create_tamper_rule tool
type CreateTamperRuleInput struct {
	CollectionID string `json:"collection_id" jsonschema:"required,ID of the tamper rule collection"`
	Name         string `json:"name" jsonschema:"required,Name for the new rule"`
	Section      string `json:"section" jsonschema:"required,Section to match: requestAll requestHeader requestBody requestPath requestQuery requestMethod requestFirstLine requestSNI responseAll responseHeader responseBody responseFirstLine responseStatusCode"`
	// Operation selects the mode. When omitted the section's default mode
	// is used with the legacy Match/Replace fields below.
	Operation *TamperOperation `json:"operation,omitempty" jsonschema:"Operation mode and its parameters. Omit to use updateRaw with match/replace."`
	Match     string           `json:"match,omitempty" jsonschema:"Regex pattern to match. Legacy shorthand for operation.match."`
	Replace   string           `json:"replace,omitempty" jsonschema:"Replacement string. Legacy shorthand for operation.value."`
	Condition *string          `json:"condition,omitempty" jsonschema:"HTTPQL filter condition"`
	Sources   []string         `json:"sources,omitempty" jsonschema:"Traffic sources: INTERCEPT REPLAY AUTOMATE IMPORT PLUGIN WORKFLOW SAMPLE"`
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

		section, err := buildTamperSection(
			input.Section,
			resolveTamperOperation(
				input.Operation, input.Match, input.Replace,
			),
		)
		if err != nil {
			return nil, CreateTamperRuleOutput{}, err
		}

		sources := make([]gen.Source, 0, len(input.Sources))
		for _, s := range input.Sources {
			sources = append(sources, gen.Source(s))
		}

		var cond map[string]any
		if input.Condition != nil {
			cond = map[string]any{
				"HTTPQL": map[string]any{"code": *input.Condition},
			}
		}

		vars := &createTamperRuleVars{
			Input: createTamperRuleGQLInput{
				CollectionId: input.CollectionID,
				Name:         input.Name,
				Section:      section,
				Condition:    cond,
				Sources:      sources,
			},
		}

		gqlReq := &gql.Request{
			OpName:    "CreateTamperRule",
			Query:     createTamperRuleMutation,
			Variables: vars,
		}
		data := &createTamperRuleResp{}
		gqlResp := &gql.Response{Data: data}
		if err := client.GraphQL.MakeRequest(
			ctx, gqlReq, gqlResp,
		); err != nil {
			return nil, CreateTamperRuleOutput{}, err
		}

		payload := data.CreateTamperRule
		if payload.Error != nil {
			return nil, CreateTamperRuleOutput{}, fmt.Errorf(
				"create tamper rule failed: %s",
				payload.Error.Typename,
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
			`section (required), operation (mode + params), ` +
			`condition (HTTPQL filter), sources (traffic sources). ` +
			`Prefer operation.kind=updateValue/add/remove with a header ` +
			`or query param name over hand-written regex. ` +
			tamperSectionDoc() + `. ` +
			`Legacy match/replace still work and mean updateRaw.`,
		Annotations: writeAnn(false, false, false),
	}, createTamperRuleHandler(client))
}
