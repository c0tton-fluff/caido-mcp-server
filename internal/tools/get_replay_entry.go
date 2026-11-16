package tools

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/httputil"
	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/replay"
	caido "github.com/caido-community/sdk-go"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GetReplayEntryInput is the input for the get_replay_entry tool
type GetReplayEntryInput struct {
	ID         string `json:"id" jsonschema:"required,Replay entry ID"`
	BodyOffset int    `json:"bodyOffset,omitempty" jsonschema:"Body byte offset"`
	BodyLimit  int    `json:"bodyLimit,omitempty" jsonschema:"Body byte limit (default 2000)"`
	// BodyEncoding forces how the response body is rendered. Empty means
	// auto: UTF-8 bodies stay text, binary bodies become base64.
	BodyEncoding string `json:"bodyEncoding,omitempty" jsonschema:"Body text or base64: auto (default; text for UTF-8 bodies, base64 for binary), text, base64"`
	// SessionKind selects the replay kind to read. Auto (default) reads the
	// entry as HTTP first and falls back to WebSocket.
	SessionKind string `json:"sessionKind,omitempty" jsonschema:"Entry kind: auto (default; tries HTTP then WS), HTTP, or WS"`
	// MessageLimit caps how many recorded WebSocket frames are returned.
	MessageLimit int `json:"messageLimit,omitempty" jsonschema:"WebSocket frames to return (default 20, max 100)"`
}

// GetReplayEntryOutput is the output of the get_replay_entry tool
type GetReplayEntryOutput struct {
	ID          string                  `json:"id"`
	Request     string                  `json:"request"`
	Response    *httputil.ParsedMessage `json:"response,omitempty"`
	Host        string                  `json:"host,omitempty"`
	Port        int                     `json:"port,omitempty"`
	IsTLS       bool                    `json:"isTls,omitempty"`
	StatusCode  int                     `json:"statusCode,omitempty"`
	RoundtripMs int                     `json:"roundtripMs,omitempty"`

	// WebSocket replay entries add the kind, the pending draft and every
	// frame recorded on the entry. Request/Response then describe the
	// handshake rather than a request/response pair.
	Kind              string        `json:"kind,omitempty"`
	EntryError        string        `json:"entryError,omitempty"`
	StreamID          string        `json:"streamId,omitempty"`
	Draft             *WsDraftView  `json:"draft,omitempty"`
	Messages          []WsFrameView `json:"messages,omitempty"`
	MessageCount      int           `json:"messageCount,omitempty"`
	MessagesTruncated bool          `json:"messagesTruncated,omitempty"`
}

// getReplayEntryHandler creates the handler function
func getReplayEntryHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, GetReplayEntryInput) (*mcp.CallToolResult, GetReplayEntryOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input GetReplayEntryInput,
	) (*mcp.CallToolResult, GetReplayEntryOutput, error) {
		if input.ID == "" {
			return nil, GetReplayEntryOutput{}, fmt.Errorf(
				"entry ID is required",
			)
		}

		switch strings.ToUpper(strings.TrimSpace(input.SessionKind)) {
		case "WS", "WEBSOCKET":
			out, err := wsReplayEntry(ctx, client, input)
			return nil, out, err
		case "HTTP":
			out, err := httpReplayEntry(ctx, client, input)
			return nil, out, err
		case "", "AUTO":
			out, err := httpReplayEntry(ctx, client, input)
			if err == nil {
				return nil, out, nil
			}
			// A WebSocket entry is invisible to the HTTP query: Caido answers
			// null, which the HTTP path reports as "entry not found".
			wsOut, wsErr := wsReplayEntry(ctx, client, input)
			if wsErr == nil {
				return nil, wsOut, nil
			}
			if isNotFound(err) {
				return nil, GetReplayEntryOutput{}, wsErr
			}
			return nil, GetReplayEntryOutput{}, err
		default:
			return nil, GetReplayEntryOutput{}, fmt.Errorf(
				"sessionKind must be one of [auto http ws], got %q",
				input.SessionKind,
			)
		}
	}
}

func isNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "not found")
}

// httpReplayEntry reads an HTTP replay entry through sdk-go.
func httpReplayEntry(
	ctx context.Context, client *caido.Client, input GetReplayEntryInput,
) (GetReplayEntryOutput, error) {
	entry, err := client.Replay.GetEntry(ctx, input.ID, "")
	if err != nil {
		return GetReplayEntryOutput{}, replay.ExplainUnsupportedKind(
			err, "replay entry", input.ID,
		)
	}

	if entry == nil {
		return GetReplayEntryOutput{}, fmt.Errorf("entry not found")
	}

	bodyLimit := input.BodyLimit
	if bodyLimit == 0 {
		bodyLimit = httputil.DefaultBodyLimit
	}
	encoding, err := httputil.ParseEncoding(input.BodyEncoding)
	if err != nil {
		return GetReplayEntryOutput{}, err
	}

	output := GetReplayEntryOutput{ID: entry.ID, Kind: "http"}

	if entry.Raw != "" {
		decoded, decErr := base64.StdEncoding.DecodeString(
			entry.Raw,
		)
		if decErr == nil {
			output.Request = string(decoded)
		}
	}

	output.Host = entry.Connection.Host
	output.Port = entry.Connection.Port
	output.IsTLS = entry.Connection.IsTLS

	if entry.Request != nil && entry.Request.Response != nil {
		r := entry.Request.Response
		output.StatusCode = r.StatusCode
		output.RoundtripMs = r.RoundtripTime
		output.Response = httputil.ParseBase64With(
			r.Raw, true, true,
			input.BodyOffset, bodyLimit, encoding,
		)
	}

	return output, nil
}

// wsReplayEntry reads a WebSocket replay entry: the handshake it replays, the
// pending draft and the frames recorded so far.
func wsReplayEntry(
	ctx context.Context, client *caido.Client, input GetReplayEntryInput,
) (GetReplayEntryOutput, error) {
	bodyLimit := input.BodyLimit
	if bodyLimit == 0 {
		bodyLimit = httputil.DefaultBodyLimit
	}
	encoding, err := httputil.ParseEncoding(input.BodyEncoding)
	if err != nil {
		return GetReplayEntryOutput{}, err
	}

	entry, err := replay.GetWsEntry(ctx, client, input.ID)
	if err != nil {
		return GetReplayEntryOutput{}, err
	}
	if entry.EntryMissing {
		return GetReplayEntryOutput{}, fmt.Errorf("entry not found")
	}
	if !entry.IsWebSocket {
		return GetReplayEntryOutput{}, fmt.Errorf(
			"replay entry %s is a %s entry, not a WebSocket entry",
			input.ID, strings.TrimPrefix(entry.Typename, "ReplayEntry"),
		)
	}

	output := GetReplayEntryOutput{
		ID:         entry.ID,
		Kind:       "ws",
		EntryError: entry.Error,
		StreamID:   entry.StreamID,
		Host:       entry.Handshake.Host,
		Port:       entry.Handshake.Port,
		IsTLS:      entry.Handshake.IsTLS,
	}
	if raw := entry.Handshake.RequestRaw; raw != "" {
		if decoded, decErr := base64.StdEncoding.DecodeString(raw); decErr == nil {
			output.Request = string(decoded)
		}
	}
	output.StatusCode = entry.Handshake.StatusCode
	output.RoundtripMs = entry.Handshake.RoundtripMs
	if entry.Handshake.ResponseRaw != "" {
		output.Response = httputil.ParseBase64With(
			entry.Handshake.ResponseRaw, true, true,
			input.BodyOffset, bodyLimit, encoding,
		)
	}

	if entry.Draft != nil {
		body, bodyEncoding, size, truncated := renderWsFrame(
			entry.Draft.Raw, bodyLimit, encoding,
		)
		output.Draft = &WsDraftView{
			Direction:    entry.Draft.Direction,
			Format:       entry.Draft.Format,
			Body:         body,
			BodyEncoding: bodyEncoding,
			BodySize:     size,
			Truncated:    truncated,
		}
	}

	limit := clampLimit(input.MessageLimit, 20, 100)
	output.MessageCount = len(entry.Messages)
	for i, m := range entry.Messages {
		if i >= limit {
			output.MessagesTruncated = true
			break
		}
		body, bodyEncoding, size, truncated := renderWsFrame(
			m.Raw, bodyLimit, encoding,
		)
		output.Messages = append(output.Messages, WsFrameView{
			ID:           m.ID,
			Direction:    m.Direction,
			Format:       m.Format,
			Length:       m.Length,
			Body:         body,
			BodyEncoding: bodyEncoding,
			BodySize:     size,
			Truncated:    truncated,
			CreatedAt:    m.CreatedAt,
		})
	}

	return output, nil
}

// RegisterGetReplayEntryTool registers the tool with the MCP server
func RegisterGetReplayEntryTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_get_replay_entry",
		Title:       "Get Replay Entry",
		Description: `Get replay entry with full request and response content. Use after send_request timeout to retrieve results. The response body is text when UTF-8 and base64 (bodyBase64, bodyEncoding:"base64") when binary; bodyEncoding forces either. WebSocket entries (sessionKind:"WS" or auto-detected) return the handshake, the pending draft and the frames recorded on the entry; messageLimit caps frames.`,
		InputSchema: schemaFor[GetReplayEntryInput](func(s *jsonschema.Schema) {
			if p := prop(s, "bodyEncoding"); p != nil {
				p.Enum = []any{"auto", "text", "base64"}
			}
			// sessionKind is handled case-insensitively (ws/http/auto), so it
			// stays unconstrained at the schema boundary, like
			// caido_create_replay_session.kind.
		}),
		Annotations: readOnlyAnn(),
	}, getReplayEntryHandler(client))
}
