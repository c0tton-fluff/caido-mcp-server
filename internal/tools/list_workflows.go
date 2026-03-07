package tools

import (
	"context"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ListWorkflowsInput is the input for the list_workflows tool
type ListWorkflowsInput struct{}

// WorkflowSummary is a summary of a workflow
type WorkflowSummary struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Enabled bool   `json:"enabled"`
}

// ListWorkflowsOutput is the output of the list_workflows tool
type ListWorkflowsOutput struct {
	Workflows []WorkflowSummary `json:"workflows"`
}

// listWorkflowsHandler creates the handler function
func listWorkflowsHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, ListWorkflowsInput) (*mcp.CallToolResult, ListWorkflowsOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input ListWorkflowsInput,
	) (*mcp.CallToolResult, ListWorkflowsOutput, error) {
		resp, err := client.Workflows.List(ctx)
		if err != nil {
			return nil, ListWorkflowsOutput{}, err
		}

		output := ListWorkflowsOutput{
			Workflows: make(
				[]WorkflowSummary, 0,
				len(resp.Workflows),
			),
		}

		for _, w := range resp.Workflows {
			output.Workflows = append(
				output.Workflows, WorkflowSummary{
					ID:      w.Id,
					Name:    w.Name,
					Kind:    string(w.Kind),
					Enabled: w.Enabled,
				},
			)
		}

		return nil, output, nil
	}
}

// RegisterListWorkflowsTool registers the tool
func RegisterListWorkflowsTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "caido_list_workflows",
		Description: `List automation workflows. ` +
			`Returns id/name/kind/enabled.`,
	}, listWorkflowsHandler(client))
}
