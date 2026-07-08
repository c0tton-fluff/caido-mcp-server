package tools

import (
	"context"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ListReplaySessionsInput is the input for the list_replay_sessions tool
type ListReplaySessionsInput struct{}

// ReplaySessionSummary is a summary of a replay session
type ReplaySessionSummary struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	ActiveEntryID *string `json:"activeEntryId,omitempty"`
}

// ListReplaySessionsOutput is the output of the list_replay_sessions tool
type ListReplaySessionsOutput struct {
	Sessions []ReplaySessionSummary `json:"sessions"`
}

// listReplaySessionsHandler creates the handler function
func listReplaySessionsHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, ListReplaySessionsInput) (*mcp.CallToolResult, ListReplaySessionsOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input ListReplaySessionsInput,
	) (*mcp.CallToolResult, ListReplaySessionsOutput, error) {
		sessions, err := client.Replay.ListSessionSummaries(ctx, nil)
		if err != nil {
			return nil, ListReplaySessionsOutput{}, err
		}

		output := ListReplaySessionsOutput{
			Sessions: make([]ReplaySessionSummary, 0, len(sessions)),
		}

		for _, s := range sessions {
			summary := ReplaySessionSummary{ID: s.ID, Name: s.Name}
			if s.ActiveEntryID != "" {
				id := s.ActiveEntryID
				summary.ActiveEntryID = &id
			}
			output.Sessions = append(output.Sessions, summary)
		}

		return nil, output, nil
	}
}

// RegisterListReplaySessionsTool registers the tool with the MCP server
func RegisterListReplaySessionsTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_list_replay_sessions",
		Description: `List replay sessions. Returns id and name for each session.`,
		InputSchema: map[string]any{"type": "object"},
		Annotations: readOnlyAnn(),
	}, listReplaySessionsHandler(client))
}
