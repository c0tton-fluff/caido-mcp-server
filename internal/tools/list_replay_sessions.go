package tools

import (
	"context"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/replay"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ListReplaySessionsInput is the input for the list_replay_sessions tool
type ListReplaySessionsInput struct{}

// ReplaySessionSummary is a summary of a replay session
type ReplaySessionSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Kind is http, ws or pipeline (Caido 0.58+ race/sequence sessions).
	Kind           string  `json:"kind"`
	ActiveEntryID  *string `json:"activeEntryId,omitempty"`
	CollectionID   string  `json:"collectionId,omitempty"`
	CollectionName string  `json:"collectionName,omitempty"`
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
		sessions, err := replay.ListSessions(ctx, client)
		if err != nil {
			return nil, ListReplaySessionsOutput{}, err
		}

		output := ListReplaySessionsOutput{
			Sessions: make([]ReplaySessionSummary, 0, len(sessions)),
		}

		for _, s := range sessions {
			summary := ReplaySessionSummary{
				ID:             s.ID,
				Name:           s.Name,
				Kind:           s.Kind,
				CollectionID:   s.CollectionID,
				CollectionName: s.CollectionName,
			}
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
		Title:       "List Replay Sessions",
		Description: `List replay sessions. Returns id, name, kind (http, ws, pipeline) and collection for each session.`,
		InputSchema: map[string]any{"type": "object"},
		Annotations: readOnlyAnn(),
	}, listReplaySessionsHandler(client))
}
