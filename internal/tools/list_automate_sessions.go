package tools

import (
	"context"
	"time"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ListAutomateSessionsInput is the input for the list_automate_sessions tool
type ListAutomateSessionsInput struct{}

// AutomateSessionSummary is a minimal representation of an Automate session
type AutomateSessionSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt"`
}

// ListAutomateSessionsOutput is the output of the list_automate_sessions tool
type ListAutomateSessionsOutput struct {
	Sessions []AutomateSessionSummary `json:"sessions"`
}

// listAutomateSessionsHandler creates the handler function
func listAutomateSessionsHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, ListAutomateSessionsInput) (*mcp.CallToolResult, ListAutomateSessionsOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input ListAutomateSessionsInput,
	) (*mcp.CallToolResult, ListAutomateSessionsOutput, error) {
		resp, err := client.Automate.ListSessions(ctx, nil)
		if err != nil {
			return nil, ListAutomateSessionsOutput{}, err
		}

		conn := resp.AutomateSessions
		output := ListAutomateSessionsOutput{
			Sessions: make(
				[]AutomateSessionSummary, 0, len(conn.Edges),
			),
		}

		for _, edge := range conn.Edges {
			s := edge.Node
			output.Sessions = append(
				output.Sessions, AutomateSessionSummary{
					ID:   s.Id,
					Name: s.Name,
					CreatedAt: time.UnixMilli(s.CreatedAt).Format(
						time.RFC3339,
					),
				},
			)
		}

		return nil, output, nil
	}
}

// RegisterListAutomateSessionsTool registers the tool with the MCP server
func RegisterListAutomateSessionsTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_list_automate_sessions",
		Description: `List fuzzing sessions. Returns id/name/createdAt.`,
		InputSchema: map[string]any{"type": "object"},
	}, listAutomateSessionsHandler(client))
}
