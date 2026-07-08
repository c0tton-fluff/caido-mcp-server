package tools

import (
	"context"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ListReplayCollectionsInput struct{}

type ReplayCollectionSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ListReplayCollectionsOutput struct {
	Collections []ReplayCollectionSummary `json:"collections"`
}

func listReplayCollectionsHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, ListReplayCollectionsInput) (*mcp.CallToolResult, ListReplayCollectionsOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input ListReplayCollectionsInput,
	) (*mcp.CallToolResult, ListReplayCollectionsOutput, error) {
		resp, err := client.Replay.ListCollections(ctx, nil)
		if err != nil {
			return nil, ListReplayCollectionsOutput{}, err
		}

		conn := resp.ReplaySessionCollections
		output := ListReplayCollectionsOutput{
			Collections: make(
				[]ReplayCollectionSummary, 0, len(conn.Edges),
			),
		}

		for _, edge := range conn.Edges {
			c := edge.Node
			output.Collections = append(output.Collections, ReplayCollectionSummary{
				ID:   c.Id,
				Name: c.Name,
			})
		}

		return nil, output, nil
	}
}

func RegisterListReplayCollectionsTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_list_replay_collections",
		Description: `List replay collections. Returns id and name for each collection.`,
		InputSchema: map[string]any{"type": "object"},
		Annotations: readOnlyAnn(),
	}, listReplayCollectionsHandler(client))
}
