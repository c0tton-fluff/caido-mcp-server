package tools

import (
	"context"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/httputil"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ListEnvironmentsInput is the input for the tool
type ListEnvironmentsInput struct{}

// EnvironmentVariable is a variable in an environment
type EnvironmentVariable struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Kind  string `json:"kind"`
}

// EnvironmentSummary is a summary of an environment
type EnvironmentSummary struct {
	ID        string                `json:"id"`
	Name      string                `json:"name"`
	Variables []EnvironmentVariable `json:"variables"`
}

// ListEnvironmentsOutput is the output
type ListEnvironmentsOutput struct {
	Environments []EnvironmentSummary `json:"environments"`
	GlobalID     string               `json:"globalId,omitempty"`
	SelectedID   string               `json:"selectedId,omitempty"`
}

func listEnvironmentsHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, ListEnvironmentsInput) (*mcp.CallToolResult, ListEnvironmentsOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input ListEnvironmentsInput,
	) (*mcp.CallToolResult, ListEnvironmentsOutput, error) {
		listResp, err := client.Environments.List(ctx)
		if err != nil {
			return nil, ListEnvironmentsOutput{}, err
		}

		ctxResp, err := client.Environments.GetContext(ctx)
		if err != nil {
			return nil, ListEnvironmentsOutput{}, err
		}

		output := ListEnvironmentsOutput{
			Environments: make(
				[]EnvironmentSummary, 0,
				len(listResp.Environments),
			),
		}

		if ctxResp.EnvironmentContext.Global != nil {
			output.GlobalID = ctxResp.EnvironmentContext.Global.Id
		}
		if ctxResp.EnvironmentContext.Selected != nil {
			output.SelectedID = ctxResp.EnvironmentContext.Selected.Id
		}

		for _, env := range listResp.Environments {
			summary := EnvironmentSummary{
				ID:   env.Id,
				Name: env.Name,
				Variables: make(
					[]EnvironmentVariable, 0,
					len(env.Variables),
				),
			}
			for _, v := range env.Variables {
				summary.Variables = append(
					summary.Variables,
					EnvironmentVariable{
						Name:  v.Name,
						Value: environmentValue(string(v.Kind), v.Value),
						Kind:  string(v.Kind),
					},
				)
			}
			output.Environments = append(
				output.Environments, summary,
			)
		}

		return nil, output, nil
	}
}

// environmentValue is the value shown for a variable of the given kind.
// SECRET values are withheld unless the operator opted out of redaction:
// Caido marks them secret so they stay out of view, and a listing is not a
// reason to put them in the model's context. PLAIN values are shown.
func environmentValue(kind, value string) string {
	if kind == "SECRET" && !httputil.SensitiveValuesAllowed() {
		return httputil.Redacted
	}
	return value
}

// RegisterListEnvironmentsTool registers the tool
func RegisterListEnvironmentsTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_list_environments",
		Title:       "List Environments",
		Description: `List environments and their variables, and which environment is selected. Values of SECRET variables come back as [REDACTED] (the variable exists; reference it from Replay placeholders instead of copying it). PLAIN values are shown.`,
		Annotations: readOnlyAnn(),
	}, listEnvironmentsHandler(client))
}
