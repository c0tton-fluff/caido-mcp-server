package tools

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/c0tton-fluff/caido-mcp-server/internal/caido"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// SendRequestInput is the input for the send_request tool
type SendRequestInput struct {
	// Raw HTTP request (plaintext, not base64)
	Raw string `json:"raw" jsonschema:"required,Raw HTTP request including headers and body"`
	// Target host (required if not in Host header)
	Host string `json:"host,omitempty" jsonschema:"Target host (overrides Host header)"`
	// Target port (default 443 for HTTPS, 80 for HTTP)
	Port int `json:"port,omitempty" jsonschema:"Target port (default based on TLS)"`
	// Use TLS/HTTPS (default true)
	TLS *bool `json:"tls,omitempty" jsonschema:"Use HTTPS (default true)"`
	// Replay session ID (creates new if not specified)
	SessionID string `json:"sessionId,omitempty" jsonschema:"Replay session ID (optional)"`
}

// SendRequestOutput is the output of the send_request tool
type SendRequestOutput struct {
	TaskID    string `json:"taskId"`
	SessionID string `json:"sessionId"`
	Message   string `json:"message"`
}

// parseHostFromRequest extracts host from raw HTTP request
func parseHostFromRequest(raw string) string {
	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), "host:") {
			return strings.TrimSpace(line[5:])
		}
	}
	return ""
}

// sendRequestHandler creates the handler function for the send_request tool
func sendRequestHandler(client *caido.Client) func(context.Context, *mcp.CallToolRequest, SendRequestInput) (*mcp.CallToolResult, SendRequestOutput, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input SendRequestInput) (*mcp.CallToolResult, SendRequestOutput, error) {
		if input.Raw == "" {
			return nil, SendRequestOutput{}, fmt.Errorf("raw HTTP request is required")
		}

		// Normalize line endings
		raw := strings.ReplaceAll(input.Raw, "\r\n", "\n")
		raw = strings.ReplaceAll(raw, "\n", "\r\n")

		// Ensure request ends with double CRLF
		if !strings.HasSuffix(raw, "\r\n\r\n") {
			if strings.HasSuffix(raw, "\r\n") {
				raw += "\r\n"
			} else {
				raw += "\r\n\r\n"
			}
		}

		// Determine host
		host := input.Host
		if host == "" {
			host = parseHostFromRequest(input.Raw)
		}
		if host == "" {
			return nil, SendRequestOutput{}, fmt.Errorf("host is required (provide in input or Host header)")
		}

		// Parse host:port if present
		if strings.Contains(host, ":") {
			parts := strings.Split(host, ":")
			host = parts[0]
			if input.Port == 0 && len(parts) > 1 {
				if p, err := strconv.Atoi(parts[1]); err == nil {
					input.Port = p
				}
			}
		}

		// Determine TLS
		useTLS := true
		if input.TLS != nil {
			useTLS = *input.TLS
		}

		// Determine port
		port := input.Port
		if port == 0 {
			if useTLS {
				port = 443
			} else {
				port = 80
			}
		}

		// Use specified session or create a new one
		sessionID := input.SessionID
		if sessionID == "" {
			// Create a new replay session to avoid TaskInProgressUserError
			session, err := client.CreateReplaySession(ctx)
			if err != nil {
				return nil, SendRequestOutput{}, fmt.Errorf("failed to create replay session: %w", err)
			}
			sessionID = session.ID
		}

		// Encode request as base64
		rawBase64 := base64.StdEncoding.EncodeToString([]byte(raw))

		// Create replay task input
		taskInput := caido.StartReplayTaskInput{
			Connection: caido.ConnectionInfoInput{
				Host:  host,
				Port:  port,
				IsTLS: useTLS,
			},
			Raw: rawBase64,
			Settings: caido.ReplayEntrySettingsInput{
				Placeholders:        []caido.PlaceholderInput{},
				UpdateContentLength: true,
				ConnectionClose:     false,
			},
		}

		taskID, err := client.StartReplayTask(ctx, sessionID, taskInput)
		if err != nil {
			return nil, SendRequestOutput{}, fmt.Errorf("failed to send request: %w", err)
		}

		// Wait a moment for the request to complete
		time.Sleep(100 * time.Millisecond)

		output := SendRequestOutput{
			TaskID:    taskID,
			SessionID: sessionID,
			Message:   fmt.Sprintf("Request sent to %s://%s:%d", map[bool]string{true: "https", false: "http"}[useTLS], host, port),
		}

		return nil, output, nil
	}
}

// RegisterSendRequestTool registers the tool with the MCP server
func RegisterSendRequestTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_send_request",
		Description: `Send HTTP request. Params: raw (full request), host, port, tls (default true).`,
	}, sendRequestHandler(client))
}

// Helper to URL encode
func urlEncode(s string) string {
	return url.QueryEscape(s)
}
