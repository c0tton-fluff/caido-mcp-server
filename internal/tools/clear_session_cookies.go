package tools

import (
	"context"

	"github.com/c0tton-fluff/caido-mcp-server/internal/replay"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ClearSessionCookiesInput selects which session jar to wipe.
type ClearSessionCookiesInput struct {
	SessionID string `json:"sessionId,omitempty" jsonschema:"Replay session ID. Omit to target the current default session."`
}

// ClearSessionCookiesOutput reports the operation outcome.
type ClearSessionCookiesOutput struct {
	SessionID string `json:"sessionId"`
	Cleared   bool   `json:"cleared"`
	Note      string `json:"note,omitempty"`
}

// clearSessionCookiesHandler removes the cookie jar tracked for the
// given session. The next send_request call against that session will
// start with a fresh, empty jar.
func clearSessionCookiesHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, ClearSessionCookiesInput) (*mcp.CallToolResult, ClearSessionCookiesOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input ClearSessionCookiesInput,
	) (*mcp.CallToolResult, ClearSessionCookiesOutput, error) {
		sessionID, err := replay.GetOrCreateSession(
			ctx, client, input.SessionID,
		)
		if err != nil {
			return nil, ClearSessionCookiesOutput{}, err
		}

		cleared := replay.DefaultCookieStore().Clear(sessionID)
		out := ClearSessionCookiesOutput{
			SessionID: sessionID,
			Cleared:   cleared,
		}
		if !cleared {
			out.Note = "no jar tracked for this session"
		}
		return nil, out, nil
	}
}

// RegisterClearSessionCookiesTool registers caido_clear_session_cookies
// with the MCP server.
func RegisterClearSessionCookiesTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_clear_session_cookies",
		Description: `Wipe the in-memory cookie jar for a replay session. Use to force re-login flows or recover from poisoned cookie state. Returns sessionId and cleared status.`,
		Annotations: writeAnn(false, true, false),
	}, clearSessionCookiesHandler(client))
}
