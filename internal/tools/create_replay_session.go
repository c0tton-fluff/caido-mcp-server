package tools

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/replay"
	caido "github.com/caido-community/sdk-go"
	gen "github.com/caido-community/sdk-go/graphql"
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
		// CreateReplaySessionInput.kind is a required GraphQL field per Caido 0.57 schema.
		// Force HTTP default explicitly so we don't depend on sdk-go's empty-string default,
		// which can disappear across SDK pins or when backend tightens validation.
		kind := gen.ReplaySessionKindHttp
		switch strings.ToUpper(strings.TrimSpace(input.Kind)) {
		case "", "HTTP":
			// keep default HTTP
		case "WS":
			kind = gen.ReplaySessionKindWs
		default:
			return nil, CreateReplaySessionOutput{}, fmt.Errorf(
				"invalid kind %q (allowed: HTTP, WS)", input.Kind,
			)
		}
		createInput := &gen.CreateReplaySessionInput{Kind: kind}

		if input.CollectionID != "" {
			createInput.CollectionId = &input.CollectionID
		}
		if input.RequestSourceID != "" {
			createInput.RequestSource = &gen.RequestSourceInput{
				Id: &input.RequestSourceID,
			}
		}

		sessionID, _, err := client.Replay.CreateSession(ctx, createInput)
		if err != nil {
			return nil, CreateReplaySessionOutput{}, fmt.Errorf("create session: %w", err)
		}

		output := CreateReplaySessionOutput{ID: sessionID}

		if input.Name != "" {
			if _, _, err := replay.RenameSession(
				ctx, client, sessionID, input.Name,
			); err != nil {
				// Do not leave an unnamed session behind: nothing returns its ID.
				if _, delErr := client.Replay.DeleteSessions(
					ctx, []string{sessionID},
				); delErr != nil {
					fmt.Fprintf(os.Stderr,
						"replay: failed to delete orphaned session %s: %v\n",
						sessionID, delErr)
				}
				return nil, CreateReplaySessionOutput{}, fmt.Errorf("rename session %s: %w", sessionID, err)
			}
			output.Name = input.Name
		}

		return nil, output, nil
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
