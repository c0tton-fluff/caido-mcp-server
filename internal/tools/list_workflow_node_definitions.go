package tools

import (
	"context"
	"encoding/json"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ListWorkflowNodeDefinitionsInput struct{}

type ListWorkflowNodeDefinitionsOutput struct {
	Definitions []json.RawMessage `json:"definitions"`
}

func listWorkflowNodeDefinitionsHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, ListWorkflowNodeDefinitionsInput) (*mcp.CallToolResult, ListWorkflowNodeDefinitionsOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input ListWorkflowNodeDefinitionsInput,
	) (*mcp.CallToolResult, ListWorkflowNodeDefinitionsOutput, error) {
		resp, err := client.Workflows.ListNodeDefinitions(ctx)
		if err != nil {
			return nil, ListWorkflowNodeDefinitionsOutput{}, err
		}

		out := ListWorkflowNodeDefinitionsOutput{
			Definitions: make([]json.RawMessage, 0, len(resp.WorkflowNodeDefinitions)),
		}
		for _, d := range resp.WorkflowNodeDefinitions {
			out.Definitions = append(out.Definitions, d.Raw)
		}
		return nil, out, nil
	}
}

func RegisterListWorkflowNodeDefinitionsTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "caido_list_workflow_node_definitions",
		Description: `List the available workflow node definitions (the building ` +
			`blocks for workflow graphs). Use this when constructing a definition ` +
			`for caido_create_workflow.`,
		InputSchema: map[string]any{"type": "object"},
	}, listWorkflowNodeDefinitionsHandler(client))
}
