package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ListTasksInput struct{}

type TaskSummary struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

type ListTasksOutput struct {
	Tasks []TaskSummary `json:"tasks"`
}

func listTasksHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, ListTasksInput) (*mcp.CallToolResult, ListTasksOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input ListTasksInput,
	) (*mcp.CallToolResult, ListTasksOutput, error) {
		resp, err := client.Tasks.List(ctx)
		if err != nil {
			return nil, ListTasksOutput{}, err
		}

		output := ListTasksOutput{
			Tasks: make([]TaskSummary, 0, len(resp.Tasks)),
		}
		for _, t := range resp.Tasks {
			typeName := "unknown"
			if tn := t.GetTypename(); tn != nil {
				typeName = *tn
			}
			output.Tasks = append(output.Tasks, TaskSummary{
				ID:   t.GetId(),
				Type: typeName,
			})
		}

		return nil, output, nil
	}
}

func RegisterListTasksTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_list_tasks",
		Description: `List running background tasks (replay, workflow, export).`,
		InputSchema: map[string]any{"type": "object"},
	}, listTasksHandler(client))
}

type CancelTaskInput struct {
	ID string `json:"id" jsonschema:"required,Task ID to cancel"`
}

type CancelTaskOutput struct {
	ID        string `json:"id"`
	Cancelled bool   `json:"cancelled"`
}

func cancelTaskHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, CancelTaskInput) (*mcp.CallToolResult, CancelTaskOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input CancelTaskInput,
	) (*mcp.CallToolResult, CancelTaskOutput, error) {
		if input.ID == "" {
			return nil, CancelTaskOutput{}, fmt.Errorf("task ID is required")
		}

		_, err := client.Tasks.Cancel(ctx, input.ID)
		if err != nil {
			return nil, CancelTaskOutput{}, fmt.Errorf("cancel task: %w", err)
		}

		return nil, CancelTaskOutput{ID: input.ID, Cancelled: true}, nil
	}
}

func RegisterCancelTaskTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_cancel_task",
		Description: `Cancel a running background task by ID.`,
	}, cancelTaskHandler(client))
}
