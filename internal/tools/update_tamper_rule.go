package tools

import (
	"context"
	"fmt"

	gql "github.com/Khan/genqlient/graphql"
	caido "github.com/caido-community/sdk-go"
	gen "github.com/caido-community/sdk-go/graphql"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type updateTamperRuleVars struct {
	ID    string                   `json:"id"`
	Input updateTamperRuleGQLInput `json:"input"`
}

type updateTamperRuleGQLInput struct {
	Name      string         `json:"name"`
	Section   map[string]any `json:"section"`
	Condition map[string]any `json:"condition,omitempty"`
	Sources   []gen.Source   `json:"sources"`
}

type updateTamperRuleResp struct {
	UpdateTamperRule struct {
		Rule *struct {
			Id   string `json:"id"`
			Name string `json:"name"`
		} `json:"rule"`
		Error *struct {
			Typename string `json:"__typename"`
		} `json:"error"`
	} `json:"updateTamperRule"`
}

const updateTamperRuleMutation = `
mutation UpdateTamperRule($id: ID!, $input: UpdateTamperRuleInput!) {
	updateTamperRule(id: $id, input: $input) {
		error { __typename }
		rule { id name }
	}
}`

// UpdateTamperRuleInput is the input for the update_tamper_rule tool.
type UpdateTamperRuleInput struct {
	ID      string `json:"id" jsonschema:"required,Tamper rule ID to update"`
	Name    string `json:"name" jsonschema:"required,Updated rule name"`
	Section string `json:"section" jsonschema:"required,Section to match: requestAll requestHeader requestBody requestPath requestQuery requestMethod requestFirstLine requestSNI responseAll responseHeader responseBody responseFirstLine responseStatusCode"`
	// Operation selects the mode. When omitted the section's default mode
	// is used with the legacy Match/Replace fields below.
	Operation *TamperOperation `json:"operation,omitempty" jsonschema:"Operation mode and its parameters. Omit to use updateRaw with match/replace."`
	Match     string           `json:"match,omitempty" jsonschema:"Regex pattern to match. Legacy shorthand for operation.match."`
	Replace   string           `json:"replace,omitempty" jsonschema:"Replacement string. Legacy shorthand for operation.value."`
	Condition *string          `json:"condition,omitempty" jsonschema:"HTTPQL filter condition"`
	Sources   []string         `json:"sources,omitempty" jsonschema:"Traffic sources: INTERCEPT REPLAY AUTOMATE IMPORT PLUGIN WORKFLOW SAMPLE"`
}

// UpdateTamperRuleOutput is the output of the update_tamper_rule tool.
type UpdateTamperRuleOutput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// updateTamperRuleHandler creates the handler function.
func updateTamperRuleHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, UpdateTamperRuleInput) (*mcp.CallToolResult, UpdateTamperRuleOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input UpdateTamperRuleInput,
	) (*mcp.CallToolResult, UpdateTamperRuleOutput, error) {
		if input.ID == "" {
			return nil, UpdateTamperRuleOutput{}, fmt.Errorf("id is required")
		}
		if input.Name == "" {
			return nil, UpdateTamperRuleOutput{}, fmt.Errorf("name is required")
		}
		if len(input.Name) > 200 {
			return nil, UpdateTamperRuleOutput{}, fmt.Errorf(
				"name exceeds max length of 200",
			)
		}
		if input.Condition != nil && len(*input.Condition) > 10000 {
			return nil, UpdateTamperRuleOutput{}, fmt.Errorf(
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
			return nil, UpdateTamperRuleOutput{}, err
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

		vars := &updateTamperRuleVars{
			ID: input.ID,
			Input: updateTamperRuleGQLInput{
				Name:      input.Name,
				Section:   section,
				Condition: cond,
				Sources:   sources,
			},
		}

		gqlReq := &gql.Request{
			OpName:    "UpdateTamperRule",
			Query:     updateTamperRuleMutation,
			Variables: vars,
		}
		data := &updateTamperRuleResp{}
		gqlResp := &gql.Response{Data: data}
		if err := client.GraphQL.MakeRequest(
			ctx, gqlReq, gqlResp,
		); err != nil {
			return nil, UpdateTamperRuleOutput{}, err
		}

		payload := data.UpdateTamperRule
		if payload.Error != nil {
			return nil, UpdateTamperRuleOutput{}, fmt.Errorf(
				"update tamper rule failed: %s",
				payload.Error.Typename,
			)
		}
		if payload.Rule == nil {
			return nil, UpdateTamperRuleOutput{}, fmt.Errorf(
				"update tamper rule returned no rule",
			)
		}

		return nil, UpdateTamperRuleOutput{
			ID:   payload.Rule.Id,
			Name: payload.Rule.Name,
		}, nil
	}
}

// RegisterUpdateTamperRuleTool registers the tool.
func RegisterUpdateTamperRuleTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:  "caido_update_tamper_rule",
		Title: "Update Tamper Rule",
		Description: `Update an existing Match & Replace ` +
			`(tamper) rule. Params: id (required), ` +
			`name (required), section (required), ` +
			`operation (mode + params), condition (HTTPQL filter), ` +
			`sources (traffic sources). ` +
			tamperSectionDoc() + `. ` +
			`Legacy match/replace still work and mean updateRaw. ` +
			`This is a full update; pass the complete rule state.`,
		InputSchema: schemaFor[UpdateTamperRuleInput](func(s *jsonschema.Schema) {
			if p := prop(s, "name"); p != nil {
				p.MaxLength = intPtr(200)
			}
			if p := prop(s, "condition"); p != nil {
				p.MaxLength = intPtr(10000)
			}
			if p := prop(s, "section"); p != nil {
				p.Enum = tamperSectionEnum
			}
		}),
		Annotations: writeAnn(false, true, false),
	}, updateTamperRuleHandler(client))
}
