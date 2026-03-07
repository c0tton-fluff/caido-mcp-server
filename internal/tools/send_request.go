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

	caido "github.com/caido-community/sdk-go"
	gen "github.com/caido-community/sdk-go/graphql"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var (
	defaultSessionID string
	sessionMu        sync.Mutex
)

// SendRequestInput is the input for the send_request tool
type SendRequestInput struct {
	Raw        string `json:"raw" jsonschema:"required,Raw HTTP request including headers and body"`
	Host       string `json:"host,omitempty" jsonschema:"Target host (overrides Host header)"`
	Port       int    `json:"port,omitempty" jsonschema:"Target port (default based on TLS)"`
	TLS        *bool  `json:"tls,omitempty" jsonschema:"Use HTTPS (default true)"`
	SessionID  string `json:"sessionId,omitempty" jsonschema:"Replay session ID (optional)"`
	BodyLimit  int    `json:"bodyLimit,omitempty" jsonschema:"Response body byte limit (default 2000)"`
	BodyOffset int    `json:"bodyOffset,omitempty" jsonschema:"Response body byte offset (default 0)"`
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
	pollMaxRetries = 20
	defaultBodyLim = 2000
)

func pollForResponse(
	ctx context.Context,
	client *caido.Client,
	sessionID string,
	previousEntryID string,
) (*gen.GetReplayEntryReplayEntry, error) {
	for i := 0; i < pollMaxRetries; i++ {
		sessResp, err := client.Replay.GetSession(ctx, sessionID)
		if err != nil {
			return nil, fmt.Errorf("polling failed: %w", err)
		}

		session := sessResp.ReplaySession
		if session == nil || session.ActiveEntry == nil {
			continue
		}

		if session.ActiveEntry.Id == previousEntryID {
			continue
		}

		entryResp, err := client.Replay.GetEntry(
			ctx, session.ActiveEntry.Id,
		)
		if err != nil {
			return nil, fmt.Errorf("polling failed: %w", err)
		}

		entry := entryResp.ReplayEntry
		if entry != nil &&
			entry.Request != nil &&
			entry.Request.Response != nil {
			return entry, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pollInterval):
		}
	}
	return nil, fmt.Errorf(
		"timed out after %ds waiting for response",
		pollMaxRetries/2,
	)
}

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

func createOrReuseSession(
	ctx context.Context,
	client *caido.Client,
	inputSessionID string,
) (string, error) {
	if inputSessionID != "" {
		return inputSessionID, nil
	}

	sessionMu.Lock()
	defer sessionMu.Unlock()

	if defaultSessionID != "" {
		return defaultSessionID, nil
	}

	resp, err := client.Replay.CreateSession(
		ctx, &gen.CreateReplaySessionInput{},
	)
	if err != nil {
		return "", fmt.Errorf(
			"failed to create replay session: %w", err,
		)
	}
	defaultSessionID = resp.CreateReplaySession.Session.Id
	return defaultSessionID, nil
}

// sendRequestHandler creates the handler function
func sendRequestHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, SendRequestInput) (*mcp.CallToolResult, SendRequestOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input SendRequestInput,
	) (*mcp.CallToolResult, SendRequestOutput, error) {
		if input.Raw == "" {
			return nil, SendRequestOutput{}, fmt.Errorf(
				"raw HTTP request is required",
			)
		}

		// Normalize line endings
		raw := strings.ReplaceAll(input.Raw, "\r\n", "\n")
		raw = strings.ReplaceAll(raw, "\n", "\r\n")
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
			return nil, SendRequestOutput{}, fmt.Errorf(
				"host is required (provide in input or Host header)",
			)
		}

		// Parse host:port
		if h, p, err := net.SplitHostPort(host); err == nil {
			host = h
			if input.Port == 0 {
				if port, pErr := strconv.Atoi(p); pErr == nil {
					input.Port = port
				}
			}
		}

		// Determine TLS and port
		useTLS := true
		if input.TLS != nil {
			useTLS = *input.TLS
		}
		port := input.Port
		if port == 0 {
			if useTLS {
				port = 443
			} else {
				port = 80
			}
		}

		sessionID, err := createOrReuseSession(
			ctx, client, input.SessionID,
		)
		if err != nil {
			return nil, SendRequestOutput{}, err
		}

		// Snapshot current active entry
		var previousEntryID string
		sessResp, err := client.Replay.GetSession(ctx, sessionID)
		if err == nil && sessResp.ReplaySession != nil &&
			sessResp.ReplaySession.ActiveEntry != nil {
			previousEntryID = sessResp.ReplaySession.ActiveEntry.Id
		}

		rawBase64 := base64.StdEncoding.EncodeToString([]byte(raw))

		taskInput := &gen.StartReplayTaskInput{
			Connection: &gen.ConnectionInfoInput{
				Host:  host,
				Port:  port,
				IsTLS: useTLS,
			},
			Raw: rawBase64,
			Settings: &gen.ReplayEntrySettingsInput{
				Placeholders:        []*gen.ReplayPlaceholderInput{},
				UpdateContentLength: true,
				ConnectionClose:     false,
			},
		}

		taskResp, err := client.Replay.SendRequest(
			ctx, sessionID, taskInput,
		)
		if err != nil || (taskResp.StartReplayTask != nil &&
			taskResp.StartReplayTask.Error != nil) {
			// Check for TaskInProgressUserError
			isTaskInProgress := false
			if err != nil {
				isTaskInProgress = strings.Contains(
					err.Error(), "TaskInProgressUserError",
				)
			} else if taskResp.StartReplayTask.Error != nil {
				// Check the union type
				isTaskInProgress = true
			}

			if isTaskInProgress {
				newResp, createErr := client.Replay.CreateSession(
					ctx, &gen.CreateReplaySessionInput{},
				)
				if createErr != nil {
					return nil, SendRequestOutput{}, fmt.Errorf(
						"failed to create fallback session: %w",
						createErr,
					)
				}
				sessionID = newResp.CreateReplaySession.Session.Id

				if input.SessionID == "" {
					sessionMu.Lock()
					defaultSessionID = sessionID
					sessionMu.Unlock()
				}

				previousEntryID = ""
				_, err = client.Replay.SendRequest(
					ctx, sessionID, taskInput,
				)
				if err != nil {
					return nil, SendRequestOutput{}, fmt.Errorf(
						"failed to send request (retry): %w", err,
					)
				}
			} else if err != nil {
				return nil, SendRequestOutput{}, fmt.Errorf(
					"failed to send request: %w", err,
				)
			}
		}

		output := SendRequestOutput{SessionID: sessionID}

		entry, pollErr := pollForResponse(
			ctx, client, sessionID, previousEntryID,
		)
		if pollErr != nil {
			output.Error = fmt.Sprintf(
				"poll failed: %v (use get_replay_entry to retry)",
				pollErr,
			)
			sResp, sErr := client.Replay.GetSession(ctx, sessionID)
			if sErr == nil && sResp.ReplaySession != nil &&
				sResp.ReplaySession.ActiveEntry != nil {
				output.EntryID = sResp.ReplaySession.ActiveEntry.Id
			}
			return nil, output, nil
		}

		output.EntryID = entry.Id

		bodyLimit := input.BodyLimit
		if bodyLimit == 0 {
			bodyLimit = defaultBodyLim
		}

		if entry.Request != nil {
			output.RequestID = entry.Request.Id
			output.Request = parseHTTPMessage(
				entry.Request.Raw, true, false, 0, 0,
			)
			if entry.Request.Response != nil {
				resp := entry.Request.Response
				output.StatusCode = resp.StatusCode
				output.RoundtripMs = resp.RoundtripTime
				output.Response = parseHTTPMessage(
					resp.Raw, true, true,
					input.BodyOffset, bodyLimit,
				)
			}
		}

		return nil, output, nil
	}
}

// RegisterSendRequestTool registers the tool with the MCP server
func RegisterSendRequestTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_send_request",
		Description: `Send HTTP request and return response inline. Returns statusCode, headers, body. Polls up to 10s for response. On timeout, returns entryId for manual follow-up via get_replay_entry. Params: raw (full request), host, port, tls (default true), bodyLimit (default 2000), bodyOffset (default 0).`,
	}, sendRequestHandler(client))
}
