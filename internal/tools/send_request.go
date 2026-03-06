package tools

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/c0tton-fluff/caido-mcp-server/internal/caido"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var (
	defaultSessionID string
	sessionMu        sync.Mutex
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
	RequestID   string             `json:"requestId,omitempty"`
	EntryID     string             `json:"entryId,omitempty"`
	SessionID   string             `json:"sessionId"`
	StatusCode  int                `json:"statusCode,omitempty"`
	RoundtripMs int                `json:"roundtripMs,omitempty"`
	Request     *ParsedHTTPMessage `json:"request,omitempty"`
	Response    *ParsedHTTPMessage `json:"response,omitempty"`
	Error       string             `json:"error,omitempty"`
}

const (
	pollInterval   = 500 * time.Millisecond
	pollMaxRetries = 20 // 10s total
	defaultBodyLim = 2000
)

// pollForResponse polls the replay session until a NEW response is available.
// previousEntryID is the active entry before StartReplayTask was called;
// we skip it to avoid returning stale data from the previous request.
func pollForResponse(
	ctx context.Context,
	client *caido.Client,
	sessionID string,
	previousEntryID string,
) (*caido.ReplayEntry, error) {
	for i := 0; i < pollMaxRetries; i++ {
		session, err := client.GetReplaySession(ctx, sessionID)
		if err != nil {
			return nil, fmt.Errorf("polling failed: %w", err)
		}

		if session.ActiveEntry != nil && session.ActiveEntry.ID != previousEntryID {
			entry, err := client.GetReplayEntry(ctx, session.ActiveEntry.ID)
			if err != nil {
				return nil, fmt.Errorf("polling failed: %w", err)
			}
			if entry.Request != nil && entry.Request.Response != nil {
				return entry, nil
			}
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pollInterval):
		}
	}
	return nil, fmt.Errorf("timed out after %ds waiting for response", pollMaxRetries/2)
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

		// Parse host:port if present (handles IPv6 like [::1]:8080)
		if h, p, err := net.SplitHostPort(host); err == nil {
			host = h
			if input.Port == 0 {
				if port, err := strconv.Atoi(p); err == nil {
					input.Port = port
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

		// Use specified session or lazily create/reuse default
		sessionID := input.SessionID
		if sessionID == "" {
			sessionMu.Lock()
			if defaultSessionID == "" {
				session, err := client.CreateReplaySession(ctx)
				if err != nil {
					sessionMu.Unlock()
					return nil, SendRequestOutput{}, fmt.Errorf(
						"failed to create replay session: %w", err,
					)
				}
				defaultSessionID = session.ID
			}
			sessionID = defaultSessionID
			sessionMu.Unlock()
		}

		// Snapshot current active entry to detect stale polls later
		var previousEntryID string
		if sess, err := client.GetReplaySession(ctx, sessionID); err == nil && sess.ActiveEntry != nil {
			previousEntryID = sess.ActiveEntry.ID
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

		_, err := client.StartReplayTask(ctx, sessionID, taskInput)
		if err != nil {
			// On TaskInProgressUserError, create a new session and retry
			if strings.Contains(err.Error(), "TaskInProgressUserError") {
				session, createErr := client.CreateReplaySession(ctx)
				if createErr != nil {
					return nil, SendRequestOutput{}, fmt.Errorf(
						"failed to create fallback session: %w", createErr,
					)
				}
				sessionID = session.ID

				if input.SessionID == "" {
					sessionMu.Lock()
					defaultSessionID = sessionID
					sessionMu.Unlock()
				}

				_, err = client.StartReplayTask(ctx, sessionID, taskInput)
				if err != nil {
					return nil, SendRequestOutput{}, fmt.Errorf(
						"failed to send request (retry): %w", err,
					)
				}
			} else {
				return nil, SendRequestOutput{}, fmt.Errorf(
					"failed to send request: %w", err,
				)
			}
		}

		output := SendRequestOutput{SessionID: sessionID}

		// Poll for response inline (skip previous entry to avoid stale data)
		entry, pollErr := pollForResponse(ctx, client, sessionID, previousEntryID)
		if pollErr != nil {
			output.Error = fmt.Sprintf(
				"poll failed: %v (use get_replay_entry to retry)", pollErr,
			)
			// Still try to get entryID for follow-up
			if session, sErr := client.GetReplaySession(ctx, sessionID); sErr == nil && session.ActiveEntry != nil {
				output.EntryID = session.ActiveEntry.ID
			}
			return nil, output, nil
		}

		output.EntryID = entry.ID

		// Populate request metadata
		if entry.Request != nil {
			output.RequestID = entry.Request.ID
			output.Request = parseHTTPMessage(
				entry.Request.Raw, true, false, 0, 0,
			)
			if entry.Request.Response != nil {
				resp := entry.Request.Response
				output.StatusCode = resp.StatusCode
				output.RoundtripMs = resp.RoundtripTime
				output.Response = parseHTTPMessage(
					resp.Raw, true, true, 0, defaultBodyLim,
				)
			}
		}

		return nil, output, nil
	}
}

// RegisterSendRequestTool registers the tool with the MCP server
func RegisterSendRequestTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_send_request",
		Description: `Send HTTP request and return response inline. Returns statusCode, headers, body (2KB limit). Polls up to 10s for response. On timeout, returns entryId for manual follow-up via get_replay_entry. Params: raw (full request), host, port, tls (default true).`,
	}, sendRequestHandler(client))
}
