package tools

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/c0tton-fluff/caido-mcp-server/internal/caido"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GetReplayEntryInput is the input for the get_replay_entry tool
type GetReplayEntryInput struct {
	ID string `json:"id" jsonschema:"required,Replay entry ID"`
}

// GetReplayEntryOutput is the output of the get_replay_entry tool
type GetReplayEntryOutput struct {
	ID          string             `json:"id"`
	Request     string             `json:"request"`     // Decoded request
	Response    *ParsedHTTPMessage `json:"response,omitempty"`
	Host        string             `json:"host,omitempty"`
	Port        int                `json:"port,omitempty"`
	IsTLS       bool               `json:"isTls,omitempty"`
	StatusCode  int                `json:"statusCode,omitempty"`
	RoundtripMs int                `json:"roundtripMs,omitempty"`
}

// getReplayEntryHandler creates the handler function
func getReplayEntryHandler(client *caido.Client) func(context.Context, *mcp.CallToolRequest, GetReplayEntryInput) (*mcp.CallToolResult, GetReplayEntryOutput, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input GetReplayEntryInput) (*mcp.CallToolResult, GetReplayEntryOutput, error) {
		if input.ID == "" {
			return nil, GetReplayEntryOutput{}, fmt.Errorf("entry ID is required")
		}

		entry, err := client.GetReplayEntry(ctx, input.ID)
		if err != nil {
			return nil, GetReplayEntryOutput{}, err
		}

		output := GetReplayEntryOutput{
			ID: entry.ID,
		}

		// Decode request
		if entry.Raw != "" {
			decoded, err := base64.StdEncoding.DecodeString(entry.Raw)
			if err == nil {
				output.Request = string(decoded)
			}
		}

		// Connection info
		if entry.Connection != nil {
			output.Host = entry.Connection.Host
			output.Port = entry.Connection.Port
			output.IsTLS = entry.Connection.IsTLS
		}

		// Response
		if entry.Request != nil && entry.Request.Response != nil {
			resp := entry.Request.Response
			output.StatusCode = resp.StatusCode
			output.RoundtripMs = resp.RoundtripTime
			output.Response = parseHTTPMessage(resp.Raw, true, true, 0, 0)
		}

		return nil, output, nil
	}
}

// RegisterGetReplayEntryTool registers the tool with the MCP server
func RegisterGetReplayEntryTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_get_replay_entry",
		Description: `Get replay entry with request/response content.`,
	}, getReplayEntryHandler(client))
}
