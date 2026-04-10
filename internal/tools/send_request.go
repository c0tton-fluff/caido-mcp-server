package tools

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"strconv"

	"github.com/c0tton-fluff/caido-mcp-server/internal/httputil"
	"github.com/c0tton-fluff/caido-mcp-server/internal/replay"
	caido "github.com/caido-community/sdk-go"
	gen "github.com/caido-community/sdk-go/graphql"
	"github.com/modelcontextprotocol/go-sdk/mcp"
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
	RequestID   string                  `json:"requestId,omitempty"`
	EntryID     string                  `json:"entryId,omitempty"`
	SessionID   string                  `json:"sessionId"`
	StatusCode  int                     `json:"statusCode,omitempty"`
	RoundtripMs int                     `json:"roundtripMs,omitempty"`
	Request     *httputil.ParsedMessage `json:"request,omitempty"`
	Response    *httputil.ParsedMessage `json:"response,omitempty"`
	Error       string                  `json:"error,omitempty"`
}

// isTaskInProgress checks whether the error from
// StartReplayTask is a TaskInProgressUserError.
func isTaskInProgress(
	resp *gen.StartReplayTaskResponse,
) bool {
	if resp == nil {
		return false
	}
	payload := resp.GetStartReplayTask()
	errPtr := payload.GetError()
	if errPtr == nil {
		return false
	}
	_, ok := (*errPtr).(*gen.StartReplayTaskStartReplayTaskStartReplayTaskPayloadErrorTaskInProgressUserError)
	return ok
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
		if len(input.Raw) > 1048576 {
			return nil, SendRequestOutput{}, fmt.Errorf(
				"raw request exceeds max length of 1MB",
			)
		}

		raw := httputil.NormalizeCRLF(input.Raw)

		// Determine host
		host := input.Host
		if host == "" {
			host = httputil.ParseHostHeader(input.Raw)
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

		sessionID, err := replay.GetOrCreateSession(
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
			Connection: gen.ConnectionInfoInput{
				Host:  host,
				Port:  port,
				IsTLS: useTLS,
			},
			Raw: rawBase64,
			Settings: gen.ReplayEntrySettingsInput{
				Placeholders:        []gen.ReplayPlaceholderInput{},
				UpdateContentLength: true,
				ConnectionClose:     false,
			},
		}

		taskResp, err := client.Replay.SendRequest(
			ctx, sessionID, taskInput,
		)
		if err != nil || isTaskInProgress(taskResp) {
			// Session busy or error - create a new session and retry.
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
				replay.ResetDefaultSession(sessionID)
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
		}

		output := SendRequestOutput{SessionID: sessionID}

		entry, pollErr := replay.PollForEntry(
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
			bodyLimit = httputil.DefaultBodyLimit
		}

		if entry.Request != nil {
			output.RequestID = entry.Request.Id
			output.Request = httputil.ParseBase64(
				entry.Request.Raw, true, false, 0, 0,
			)
			if entry.Request.Response != nil {
				resp := entry.Request.Response
				output.StatusCode = resp.StatusCode
				output.RoundtripMs = resp.RoundtripTime
				output.Response = httputil.ParseBase64(
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
