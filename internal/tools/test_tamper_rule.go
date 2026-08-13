package tools

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	gql "github.com/Khan/genqlient/graphql"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// testTamperRuleVars wraps the GraphQL variables. Section is a map for the
// same oneof reason documented in create_tamper_rule.go.
type testTamperRuleVars struct {
	Input testTamperRuleGQLInput `json:"input"`
}

type testTamperRuleGQLInput struct {
	Raw     string         `json:"raw"`
	Section map[string]any `json:"section"`
}

type testTamperRuleResp struct {
	TestTamperRule struct {
		Raw   *string `json:"raw"`
		Error *struct {
			Typename string `json:"__typename"`
		} `json:"error"`
	} `json:"testTamperRule"`
}

const testTamperRuleMutation = `
mutation TestTamperRule($input: TestTamperRuleInput!) {
	testTamperRule(input: $input) {
		error { __typename }
		raw
	}
}`

// TestTamperRuleInput is the input for the test_tamper_rule tool.
type TestTamperRuleInput struct {
	Raw     string `json:"raw" jsonschema:"required,Raw HTTP request or response to transform. Plain text; CRLF line endings recommended."`
	Section string `json:"section" jsonschema:"required,Section to match: requestAll requestHeader requestBody requestPath requestQuery requestMethod requestFirstLine requestSNI responseAll responseHeader responseBody responseFirstLine responseStatusCode"`
	// Operation selects the mode. When omitted the section's default mode
	// is used with the legacy Match/Replace fields below.
	Operation *TamperOperation `json:"operation,omitempty" jsonschema:"Operation mode and its parameters. Omit to use updateRaw with match/replace."`
	Match     string           `json:"match,omitempty" jsonschema:"Regex pattern to match. Legacy shorthand for operation.match."`
	Replace   string           `json:"replace,omitempty" jsonschema:"Replacement string. Legacy shorthand for operation.value."`
}

// TestTamperRuleOutput is the output of the test_tamper_rule tool.
type TestTamperRuleOutput struct {
	// Raw is the transformed request or response.
	Raw string `json:"raw"`
	// Changed reports whether the rule altered the input at all. A rule
	// that silently matches nothing returns Changed=false with Raw equal
	// to the input, which is the common authoring mistake this tool
	// exists to surface.
	Changed bool `json:"changed"`
}

// testTamperRuleHandler creates the handler function.
func testTamperRuleHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, TestTamperRuleInput) (*mcp.CallToolResult, TestTamperRuleOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input TestTamperRuleInput,
	) (*mcp.CallToolResult, TestTamperRuleOutput, error) {
		if input.Raw == "" {
			return nil, TestTamperRuleOutput{}, fmt.Errorf("raw is required")
		}
		if err := checkRawSize("raw", input.Raw); err != nil {
			return nil, TestTamperRuleOutput{}, err
		}

		section, err := buildTamperSection(
			input.Section,
			resolveTamperOperation(
				input.Operation, input.Match, input.Replace,
			),
		)
		if err != nil {
			return nil, TestTamperRuleOutput{}, err
		}

		// Blob type requires base64 encoding.
		vars := &testTamperRuleVars{
			Input: testTamperRuleGQLInput{
				Raw: base64.StdEncoding.EncodeToString(
					[]byte(input.Raw),
				),
				Section: section,
			},
		}

		gqlReq := &gql.Request{
			OpName:    "TestTamperRule",
			Query:     testTamperRuleMutation,
			Variables: vars,
		}
		data := &testTamperRuleResp{}
		gqlResp := &gql.Response{Data: data}
		if err := client.GraphQL.MakeRequest(
			ctx, gqlReq, gqlResp,
		); err != nil {
			return nil, TestTamperRuleOutput{}, err
		}

		payload := data.TestTamperRule
		if payload.Error != nil {
			return nil, TestTamperRuleOutput{}, fmt.Errorf(
				"test tamper rule failed: %s", payload.Error.Typename,
			)
		}
		if payload.Raw == nil {
			return nil, TestTamperRuleOutput{}, fmt.Errorf(
				"test tamper rule returned no result",
			)
		}

		decoded, err := base64.StdEncoding.DecodeString(*payload.Raw)
		if err != nil {
			return nil, TestTamperRuleOutput{}, fmt.Errorf(
				"decode transformed raw: %w", err,
			)
		}

		out := string(decoded)
		return nil, TestTamperRuleOutput{
			Raw:     out,
			Changed: out != input.Raw,
		}, nil
	}
}

// RegisterTestTamperRuleTool registers the tool.
func RegisterTestTamperRuleTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:  "caido_test_tamper_rule",
		Title: "Test Tamper Rule",
		Description: `Dry-run a Match & Replace (tamper) rule against a ` +
			`raw HTTP request or response and return the transformed ` +
			`result. Nothing is persisted and no traffic is sent. ` +
			`Use this to verify a rule before creating it -- the ` +
			`"changed" field reports whether the rule matched at all. ` +
			`Params: raw (required), section (required), ` +
			`operation (mode + params) or legacy match/replace.`,
		InputSchema: schemaFor[TestTamperRuleInput](func(s *jsonschema.Schema) {
			if p := prop(s, "raw"); p != nil {
				p.MaxLength = intPtr(maxRawRequestBytes)
			}
			if p := prop(s, "section"); p != nil {
				p.Enum = tamperSectionEnum
			}
		}),
		Annotations: readOnlyAnn(),
	}, testTamperRuleHandler(client))
}
