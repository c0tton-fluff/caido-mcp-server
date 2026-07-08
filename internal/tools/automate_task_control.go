package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// AutomateTaskControlInput is the input for the tool
type AutomateTaskControlInput struct {
	Action    string `json:"action" jsonschema:"required,Action: start, pause, resume, or cancel"`
	SessionID string `json:"session_id,omitempty" jsonschema:"Automate session ID (required for start)"`
	TaskID    string `json:"task_id,omitempty" jsonschema:"Automate task ID (required for pause/resume/cancel)"`
}

// AutomateTaskControlOutput is the output
type AutomateTaskControlOutput struct {
	Action string `json:"action"`
	TaskID string `json:"taskId"`
}

func automateTaskControlHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, AutomateTaskControlInput) (*mcp.CallToolResult, AutomateTaskControlOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input AutomateTaskControlInput,
	) (*mcp.CallToolResult, AutomateTaskControlOutput, error) {
		switch input.Action {
		case "start":
			if input.SessionID == "" {
				return nil, AutomateTaskControlOutput{}, fmt.Errorf(
					"session_id is required for start action",
				)
			}
			resp, err := client.Automate.StartTask(
				ctx, input.SessionID,
			)
			if err != nil {
				return nil, AutomateTaskControlOutput{}, err
			}
			return nil, AutomateTaskControlOutput{
				Action: "start",
				TaskID: resp.StartAutomateTask.AutomateTask.Id,
			}, nil

		case "pause":
			if input.TaskID == "" {
				return nil, AutomateTaskControlOutput{}, fmt.Errorf(
					"task_id is required for pause action",
				)
			}
			resp, err := client.Automate.PauseTask(
				ctx, input.TaskID,
			)
			if err != nil {
				return nil, AutomateTaskControlOutput{}, err
			}
			return nil, AutomateTaskControlOutput{
				Action: "pause",
				TaskID: resp.PauseAutomateTask.AutomateTask.Id,
			}, nil

		case "resume":
			if input.TaskID == "" {
				return nil, AutomateTaskControlOutput{}, fmt.Errorf(
					"task_id is required for resume action",
				)
			}
			resp, err := client.Automate.ResumeTask(
				ctx, input.TaskID,
			)
			if err != nil {
				return nil, AutomateTaskControlOutput{}, err
			}
			return nil, AutomateTaskControlOutput{
				Action: "resume",
				TaskID: resp.ResumeAutomateTask.AutomateTask.Id,
			}, nil

		case "cancel":
			if input.TaskID == "" {
				return nil, AutomateTaskControlOutput{}, fmt.Errorf(
					"task_id is required for cancel action",
				)
			}
			resp, err := client.Automate.CancelTask(
				ctx, input.TaskID,
			)
			if err != nil {
				return nil, AutomateTaskControlOutput{}, err
			}
			cancelID := ""
			if resp.CancelAutomateTask.CancelledId != nil {
				cancelID = *resp.CancelAutomateTask.CancelledId
			}
			return nil, AutomateTaskControlOutput{
				Action: "cancel",
				TaskID: cancelID,
			}, nil

		default:
			return nil, AutomateTaskControlOutput{}, fmt.Errorf(
				"action must be start, pause, resume, or cancel",
			)
		}
	}
}

// RegisterAutomateTaskControlTool registers the tool
func RegisterAutomateTaskControlTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_automate_task_control",
		Description: `Control fuzzing tasks. Actions: start (needs session_id), pause/resume/cancel (needs task_id).`,
		Annotations: writeAnn(false, false, false),
	}, automateTaskControlHandler(client))
}
