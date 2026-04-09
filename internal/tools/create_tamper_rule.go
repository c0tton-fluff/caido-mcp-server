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
// oneofs. genqlient v0.8.1 drops omitempty from nullable pointer
// fields, causing unset oneof variants to serialize as null.
// Maps only include keys we set, avoiding oneof violations at
// every nesting level.
type createTamperRuleVars struct {
	Input createTamperRuleGQLInput `json:"input"`
}

type createTamperRuleGQLInput struct {
	CollectionId string         `json:"collectionId"`
	Name         string         `json:"name"`
	Section      map[string]any `json:"section"`
	Condition    *string        `json:"condition,omitempty"`
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
	CollectionID string   `json:"collection_id" jsonschema:"required,ID of the tamper rule collection"`
	Name         string   `json:"name" jsonschema:"required,Name for the new rule"`
	Section      string   `json:"section" jsonschema:"required,Section to match: requestAll requestHeader requestBody requestPath requestQuery requestMethod requestFirstLine requestSNI responseAll responseHeader responseBody responseFirstLine responseStatusCode"`
	Match        string   `json:"match,omitempty" jsonschema:"Regex pattern to match"`
	Replace      string   `json:"replace,omitempty" jsonschema:"Replacement string"`
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

		section, err := buildTamperSectionMap(
			input.Section, input.Match, input.Replace,
		)
		if err != nil {
			return nil, CreateTamperRuleOutput{}, err
		}

		sources := make([]gen.Source, 0, len(input.Sources))
		for _, s := range input.Sources {
			sources = append(sources, gen.Source(s))
		}

		vars := &createTamperRuleVars{
			Input: createTamperRuleGQLInput{
				CollectionId: input.CollectionID,
				Name:         input.Name,
				Section:      section,
				Condition:    input.Condition,
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

// buildTamperSectionMap constructs the section input as a map.
// Every nested type in the tamper section chain is a GraphQL oneof
// (section, operation, matcher, replacer). Using maps ensures only
// the chosen variant is serialized -- no null fields for unset
// variants at any nesting level.
func buildTamperSectionMap(
	section, match, replace string,
) (map[string]any, error) {
	matcher := map[string]any{
		"regex": map[string]any{"regex": match},
	}
	replacer := map[string]any{
		"term": map[string]any{"term": replace},
	}
	rawOp := map[string]any{
		"raw": map[string]any{
			"matcher":  matcher,
			"replacer": replacer,
		},
	}

	wrap := func(key string, op map[string]any) map[string]any {
		return map[string]any{
			key: map[string]any{"operation": op},
		}
	}

	switch section {
	case "requestAll":
		return wrap("requestAll", rawOp), nil
	case "requestHeader":
		return wrap("requestHeader", rawOp), nil
	case "requestBody":
		return wrap("requestBody", rawOp), nil
	case "requestPath":
		return wrap("requestPath", rawOp), nil
	case "requestQuery":
		return wrap("requestQuery", rawOp), nil
	case "requestMethod":
		return wrap("requestMethod", map[string]any{
			"update": map[string]any{
				"matcher":  matcher,
				"replacer": replacer,
			},
		}), nil
	case "requestFirstLine":
		return wrap("requestFirstLine", rawOp), nil
	case "requestSNI":
		return wrap("requestSNI", rawOp), nil
	case "responseAll":
		return wrap("responseAll", rawOp), nil
	case "responseHeader":
		return wrap("responseHeader", rawOp), nil
	case "responseBody":
		return wrap("responseBody", rawOp), nil
	case "responseFirstLine":
		return wrap("responseFirstLine", rawOp), nil
	case "responseStatusCode":
		return wrap("responseStatusCode", map[string]any{
			"update": map[string]any{
				"matcher":  matcher,
				"replacer": replacer,
			},
		}), nil
	default:
		return nil, fmt.Errorf(
			"unknown section %q: use requestAll, requestHeader, "+
				"requestBody, responseAll, responseHeader, responseBody, "+
				"or other supported sections", section,
		)
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
			`section (required: requestAll/requestHeader/requestBody/` +
			`responseAll/responseHeader/responseBody/etc), ` +
			`match (regex), replace (string), ` +
			`condition (HTTPQL filter), sources (traffic sources).`,
	}, createTamperRuleHandler(client))
}
