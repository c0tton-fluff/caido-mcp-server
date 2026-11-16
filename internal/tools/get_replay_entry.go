package tools

import (
	"context"
	"encoding/base64"
	"fmt"

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

		entry, err := client.Replay.GetEntry(ctx, input.ID, "")
		if err != nil {
			return nil, GetReplayEntryOutput{}, replay.ExplainUnsupportedKind(
				err, "replay entry", input.ID,
			)
		}

		if entry == nil {
			return nil, GetReplayEntryOutput{}, fmt.Errorf(
				"entry not found",
			)
		}

		bodyLimit := input.BodyLimit
		if bodyLimit == 0 {
			bodyLimit = httputil.DefaultBodyLimit
		}
		encoding, err := httputil.ParseEncoding(input.BodyEncoding)
		if err != nil {
			return nil, GetReplayEntryOutput{}, err
		}

		output := GetReplayEntryOutput{ID: entry.ID}

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

		return nil, output, nil
	}
}

// RegisterGetReplayEntryTool registers the tool with the MCP server
func RegisterGetReplayEntryTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_get_replay_entry",
		Title:       "Get Replay Entry",
		Description: `Get replay entry with full request and response content. Use after send_request timeout to retrieve results. The response body is text when UTF-8 and base64 (with bodyEncoding:"base64", bodySize, bodyOffset) when binary; bodyOffset/bodyLimit count bytes and bodyEncoding can force text or base64.`,
		InputSchema: schemaFor[GetReplayEntryInput](func(s *jsonschema.Schema) {
			if p := prop(s, "bodyEncoding"); p != nil {
				p.Enum = []any{"auto", "text", "base64"}
			}
		}),
		Annotations: readOnlyAnn(),
	}, getReplayEntryHandler(client))
}
