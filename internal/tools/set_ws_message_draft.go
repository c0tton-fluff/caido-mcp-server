package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/httputil"
	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/replay"
	caido "github.com/caido-community/sdk-go"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// SetWsMessageDraftInput is the input for caido_set_ws_message_draft.
type SetWsMessageDraftInput struct {
	ID              string `json:"id" jsonschema:"required,WebSocket replay entry ID (from caido_create_replay_session or caido_get_replay_entry)"`
	Payload         string `json:"payload" jsonschema:"required,Message payload (text, or base64 when payloadEncoding is base64)"`
	PayloadEncoding string `json:"payloadEncoding,omitempty" jsonschema:"Payload is text (default) or base64"`
	Direction       string `json:"direction,omitempty" jsonschema:"Frame direction: SERVER (default) or CLIENT"`
	Format          string `json:"format,omitempty" jsonschema:"Frame format: TEXT (default), BINARY, PING, PONG or CLOSE"`
	ServerTimeoutMs int    `json:"serverTimeoutMs,omitempty" jsonschema:"How long Caido waits for the server per message, in ms (default 5000, max 120000)"`
}

// SetWsMessageDraftOutput is the output of caido_set_ws_message_draft.
type SetWsMessageDraftOutput struct {
	EntryID string      `json:"entryId"`
	Draft   WsDraftView `json:"draft"`
	Applied bool        `json:"applied"`
}

func setWsMessageDraftHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, SetWsMessageDraftInput) (*mcp.CallToolResult, SetWsMessageDraftOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input SetWsMessageDraftInput,
	) (*mcp.CallToolResult, SetWsMessageDraftOutput, error) {
		if input.ID == "" {
			return nil, SetWsMessageDraftOutput{}, fmt.Errorf("id is required")
		}
		if err := checkRawSize("payload", input.Payload); err != nil {
			return nil, SetWsMessageDraftOutput{}, err
		}
		direction, err := wsDirection(input.Direction)
		if err != nil {
			return nil, SetWsMessageDraftOutput{}, err
		}
		format, err := wsFormat(input.Format)
		if err != nil {
			return nil, SetWsMessageDraftOutput{}, err
		}
		encoding, err := wsPayloadEncoding(input.PayloadEncoding)
		if err != nil {
			return nil, SetWsMessageDraftOutput{}, err
		}
		raw, err := encodeWsFrame(input.Payload, encoding)
		if err != nil {
			return nil, SetWsMessageDraftOutput{}, err
		}
		if input.ServerTimeoutMs < 0 || input.ServerTimeoutMs > 120000 {
			return nil, SetWsMessageDraftOutput{}, fmt.Errorf(
				"serverTimeoutMs must be between 0 and 120000, got %d",
				input.ServerTimeoutMs,
			)
		}

		draft, err := replay.SetWsDraft(ctx, client, input.ID, replay.WsDraftInput{
			Raw:             raw,
			Direction:       direction,
			Format:          format,
			ServerTimeoutMs: input.ServerTimeoutMs,
		})
		if err != nil {
			return nil, SetWsMessageDraftOutput{}, err
		}

		body, bodyEncoding, size, truncated := renderWsFrame(
			draft.Raw, 0, encodingForDraft(input.PayloadEncoding),
		)
		return nil, SetWsMessageDraftOutput{
			EntryID: input.ID,
			Draft: WsDraftView{
				Direction:    draft.Direction,
				Format:       draft.Format,
				Body:         body,
				BodyEncoding: bodyEncoding,
				BodySize:     size,
				Truncated:    truncated,
			},
			Applied: true,
		}, nil
	}
}

// wsDirection normalizes the direction argument. Caido 0.59.0 masks the frame
// according to this value: SERVER produces a masked frame, which is what a
// target server accepts as a client message; CLIENT produces an unmasked
// server-to-client frame.
func wsDirection(v string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(v)) {
	case "":
		return replay.DirectionServer, nil
	case replay.DirectionClient:
		return replay.DirectionClient, nil
	case replay.DirectionServer:
		return replay.DirectionServer, nil
	}
	return "", fmt.Errorf(
		"direction must be one of [CLIENT SERVER], got %q", v,
	)
}

// wsFormat normalizes the frame format argument.
func wsFormat(v string) (string, error) {
	f := strings.ToUpper(strings.TrimSpace(v))
	switch f {
	case "":
		return "TEXT", nil
	case "TEXT", "BINARY", "PING", "PONG", "CLOSE":
		return f, nil
	}
	return "", fmt.Errorf(
		"format must be one of [TEXT BINARY PING PONG CLOSE], got %q", v,
	)
}

// wsPayloadEncoding normalizes the payload encoding argument.
func wsPayloadEncoding(v string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "text":
		return "text", nil
	case "base64":
		return "base64", nil
	}
	return "", fmt.Errorf("payloadEncoding must be [text base64], got %q", v)
}

func encodingForDraft(v string) httputil.Encoding {
	if strings.EqualFold(strings.TrimSpace(v), "base64") {
		return httputil.EncodingBase64
	}
	return httputil.EncodingAuto
}

// RegisterSetWsMessageDraftTool registers the tool with the MCP server.
func RegisterSetWsMessageDraftTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:  "caido_set_ws_message_draft",
		Title: "Set WebSocket Message Draft",
		Description: `Write the pending message of a WebSocket replay entry, so it can be sent from ` +
			`the Caido UI (or read back by an agent before sending). Payload is text by default; set ` +
			`payloadEncoding:"base64" for binary frames (BINARY/PING/PONG/CLOSE). ` +
			`Direction follows Caido's masking: SERVER (default) is sent as a masked client frame, ` +
			`which a target server accepts; CLIENT is sent unmasked as a server frame. ` +
			`The draft is returned as stored, so a normalized value is visible immediately.`,
		InputSchema: schemaFor[SetWsMessageDraftInput](func(s *jsonschema.Schema) {
			if p := prop(s, "payloadEncoding"); p != nil {
				p.Enum = []any{"text", "base64"}
			}
			if p := prop(s, "direction"); p != nil {
				p.Enum = []any{"CLIENT", "SERVER"}
			}
			if p := prop(s, "format"); p != nil {
				p.Enum = []any{"TEXT", "BINARY", "PING", "PONG", "CLOSE"}
			}
		}),
		Annotations: writeAnn(false, true, false),
	}, setWsMessageDraftHandler(client))
}
