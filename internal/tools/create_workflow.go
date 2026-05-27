package tools

import (
	"context"
	"encoding/json"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	gen "github.com/caido-community/sdk-go/graphql"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type CreateWorkflowInput struct {
	Definition string `json:"definition" jsonschema:"required,Workflow node-graph definition as a JSON string (see caido_get_workflow output for the schema)"`
	Global     bool   `json:"global,omitempty" jsonschema:"Create as a global workflow shared across projects (default false)"`
}

type CreateWorkflowOutput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func createWorkflowHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, CreateWorkflowInput) (*mcp.CallToolResult, CreateWorkflowOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input CreateWorkflowInput,
	) (*mcp.CallToolResult, CreateWorkflowOutput, error) {
		if input.Definition == "" {
			return nil, CreateWorkflowOutput{}, fmt.Errorf("definition is required")
		}
		if !json.Valid([]byte(input.Definition)) {
			return nil, CreateWorkflowOutput{}, fmt.Errorf("definition is not valid JSON")
		}

		resp, err := client.Workflows.Create(ctx, &gen.CreateWorkflowInput{
			Definition: json.RawMessage(input.Definition),
			Global:     input.Global,
		})
		if err != nil {
			return nil, CreateWorkflowOutput{}, err
		}

		payload := resp.CreateWorkflow
		if payload.Error != nil {
			errType := "unknown"
			if tn := (*payload.Error).GetTypename(); tn != nil {
				errType = *tn
			}
			return nil, CreateWorkflowOutput{}, fmt.Errorf("create workflow failed: %s", errType)
		}
		if payload.Workflow == nil {
			return nil, CreateWorkflowOutput{}, fmt.Errorf("create workflow returned no workflow")
		}

		return nil, CreateWorkflowOutput{
			ID:   payload.Workflow.Id,
			Name: payload.Workflow.Name,
		}, nil
	}
}

func RegisterCreateWorkflowTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "caido_create_workflow",
		Description: `Create a workflow from a node-graph definition (JSON). ` +
			`Fetch an existing workflow with caido_get_workflow to learn the ` +
			`definition schema, and list available nodes with ` +
			`caido_list_workflow_node_definitions.`,
	}, createWorkflowHandler(client))
}
