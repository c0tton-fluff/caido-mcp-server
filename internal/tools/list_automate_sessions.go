package tools

import (
	"context"
	"time"

	"github.com/c0tton-fluff/caido-mcp-server/internal/caido"
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

// listAutomateSessionsHandler creates the handler function for the list_automate_sessions tool
func listAutomateSessionsHandler(client *caido.Client) func(context.Context, *mcp.CallToolRequest, ListAutomateSessionsInput) (*mcp.CallToolResult, ListAutomateSessionsOutput, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input ListAutomateSessionsInput) (*mcp.CallToolResult, ListAutomateSessionsOutput, error) {
		result, err := client.ListAutomateSessions(ctx)
		if err != nil {
			return nil, ListAutomateSessionsOutput{}, err
		}

		output := ListAutomateSessionsOutput{
			Sessions: make([]AutomateSessionSummary, 0, len(result.AutomateSessions.Edges)),
		}

		for _, edge := range result.AutomateSessions.Edges {
			s := edge.Node
			output.Sessions = append(output.Sessions, AutomateSessionSummary{
				ID:        s.ID,
				Name:      s.Name,
				CreatedAt: s.CreatedAt.Time().Format(time.RFC3339),
			})
		}

		return nil, output, nil
	}
}

// RegisterListAutomateSessionsTool registers the tool with the MCP server
func RegisterListAutomateSessionsTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_list_automate_sessions",
		Description: `List fuzzing sessions. Returns id/name/createdAt.`,
	}, listAutomateSessionsHandler(client))
}
