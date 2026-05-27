package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type GetReplaySessionInput struct {
	ID string `json:"id" jsonschema:"required,Replay session ID"`
}

type GetReplaySessionOutput struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	CollectionID   string `json:"collectionId"`
	CollectionName string `json:"collectionName"`
	ActiveEntryID  string `json:"activeEntryId,omitempty"`
	EntryCount     int    `json:"entryCount"`
}

func getReplaySessionHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, GetReplaySessionInput) (*mcp.CallToolResult, GetReplaySessionOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input GetReplaySessionInput,
	) (*mcp.CallToolResult, GetReplaySessionOutput, error) {
		resp, err := client.Replay.GetSession(ctx, input.ID)
		if err != nil {
			return nil, GetReplaySessionOutput{}, err
		}

		s := resp.ReplaySession
		if s == nil {
			return nil, GetReplaySessionOutput{}, fmt.Errorf("replay session not found")
		}

		out := GetReplaySessionOutput{
			ID:             s.Id,
			Name:           s.Name,
			CollectionID:   s.Collection.Id,
			CollectionName: s.Collection.Name,
			EntryCount:     len(s.Entries.Edges),
		}
		if s.ActiveEntry != nil {
			out.ActiveEntryID = s.ActiveEntry.Id
		}
		return nil, out, nil
	}
}

func RegisterGetReplaySessionTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "caido_get_replay_session",
		Description: `Get a replay session by ID: name, collection, active entry, and ` +
			`how many entries it contains.`,
	}, getReplaySessionHandler(client))
}
