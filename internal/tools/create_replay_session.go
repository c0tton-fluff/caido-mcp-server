package tools

import (
	"cmp"
	"context"
	"fmt"
	"strings"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/replay"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type CreateReplaySessionInput struct {
	Name            string `json:"name,omitempty" jsonschema:"Session name (applied via rename after creation)"`
	CollectionID    string `json:"collectionId,omitempty" jsonschema:"Collection ID to assign the session to"`
	RequestSourceID string `json:"requestSourceId,omitempty" jsonschema:"Existing request ID to seed the session with"`
	Kind            string `json:"kind,omitempty" jsonschema:"Session kind - allowed values: HTTP (default) or WS for WebSocket replay"`
}

type CreateReplaySessionOutput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func createReplaySessionHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, CreateReplaySessionInput) (*mcp.CallToolResult, CreateReplaySessionOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input CreateReplaySessionInput,
	) (*mcp.CallToolResult, CreateReplaySessionOutput, error) {
		// kind is required by the CreateReplaySessionInput schema; default HTTP.
		kind := cmp.Or(strings.ToUpper(strings.TrimSpace(input.Kind)), "HTTP")
		if kind != "HTTP" && kind != "WS" {
			return nil, CreateReplaySessionOutput{}, fmt.Errorf(
				"invalid kind %q (allowed: HTTP, WS)", input.Kind,
			)
		}
		createInput := map[string]any{"kind": kind}
		if input.CollectionID != "" {
			createInput["collectionId"] = input.CollectionID
		}
		if input.RequestSourceID != "" {
			createInput["requestSource"] = map[string]any{"id": input.RequestSourceID}
		}

		sessionID, err := replay.CreateNamedSession(ctx, client, createInput, input.Name)
		if err != nil {
			return nil, CreateReplaySessionOutput{}, fmt.Errorf("create session: %w", err)
		}
		return nil, CreateReplaySessionOutput{ID: sessionID, Name: input.Name}, nil
	}
}

func RegisterCreateReplaySessionTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_create_replay_session",
		Title:       "Create Replay Session",
		Description: `Create a named replay session. Optionally seed it with an existing request and assign to a collection. To send a new raw request as its own titled session in a collection, prefer caido_send_request with collectionId and name (one call).`,
		Annotations: writeAnn(false, false, false),
	}, createReplaySessionHandler(client))
}
