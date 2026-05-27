package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type GetWorkflowInput struct {
	ID string `json:"id" jsonschema:"required,Workflow ID"`
}

type GetWorkflowOutput struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Enabled    bool   `json:"enabled"`
	Global     bool   `json:"global"`
	ReadOnly   bool   `json:"readOnly"`
	Definition string `json:"definition"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
}

func getWorkflowHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, GetWorkflowInput) (*mcp.CallToolResult, GetWorkflowOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input GetWorkflowInput,
	) (*mcp.CallToolResult, GetWorkflowOutput, error) {
		resp, err := client.Workflows.Get(ctx, input.ID)
		if err != nil {
			return nil, GetWorkflowOutput{}, err
		}

		wf := resp.Workflow
		if wf == nil {
			return nil, GetWorkflowOutput{}, fmt.Errorf("workflow not found")
		}

		return nil, GetWorkflowOutput{
			ID:         wf.Id,
			Name:       wf.Name,
			Kind:       string(wf.Kind),
			Enabled:    wf.Enabled,
			Global:     wf.Global,
			ReadOnly:   wf.ReadOnly,
			Definition: string(wf.Definition),
			CreatedAt:  wf.CreatedAt,
			UpdatedAt:  wf.UpdatedAt,
		}, nil
	}
}

func RegisterGetWorkflowTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "caido_get_workflow",
		Description: `Get a workflow by ID including its full node-graph ` +
			`definition (JSON). Use this to inspect or clone an existing ` +
			`workflow before editing.`,
	}, getWorkflowHandler(client))
}
