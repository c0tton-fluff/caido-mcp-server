package tools

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/c0tton-fluff/caido-mcp-server/internal/httputil"
	"github.com/c0tton-fluff/caido-mcp-server/internal/replay"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type EditRequestInput struct {
	RequestID     string            `json:"requestId" jsonschema:"required,Source request ID to modify"`
	Method        string            `json:"method,omitempty" jsonschema:"Override HTTP method"`
	Path          string            `json:"path,omitempty" jsonschema:"Override request path"`
	SetHeaders    map[string]string `json:"setHeaders,omitempty" jsonschema:"Headers to add or replace"`
	RemoveHeaders []string          `json:"removeHeaders,omitempty" jsonschema:"Headers to remove"`
	Body          string            `json:"body,omitempty" jsonschema:"Override request body"`
	Host          string            `json:"host,omitempty" jsonschema:"Override target host"`
	Port          int               `json:"port,omitempty" jsonschema:"Override target port"`
	TLS           *bool             `json:"tls,omitempty" jsonschema:"Override TLS setting"`
	SessionID     string            `json:"sessionId,omitempty" jsonschema:"Replay session ID"`
	BodyLimit     int               `json:"bodyLimit,omitempty" jsonschema:"Response body byte limit (default 2000)"`
	BodyOffset    int               `json:"bodyOffset,omitempty" jsonschema:"Response body byte offset (default 0)"`
}

func replaceMethod(raw, newMethod string) string {
	idx := strings.Index(raw, "\r\n")
	if idx < 0 {
		return raw
	}
	startLine := raw[:idx]
	rest := raw[idx:]

	parts := strings.SplitN(startLine, " ", 3)
	if len(parts) < 2 {
		return raw
	}

	if len(parts) == 2 {
		return newMethod + " " + parts[1] + rest
	}
	return newMethod + " " + parts[1] + " " + parts[2] + rest
}

func replacePath(raw, newPath string) string {
	idx := strings.Index(raw, "\r\n")
	if idx < 0 {
		return raw
	}
	startLine := raw[:idx]
	rest := raw[idx:]

	parts := strings.SplitN(startLine, " ", 3)
	if len(parts) < 2 {
		return raw
	}

	if len(parts) == 2 {
		return parts[0] + " " + newPath + rest
	}
	return parts[0] + " " + newPath + " " + parts[2] + rest
}

func removeHeader(raw, name string) string {
	lowerName := strings.ToLower(name)
	separator := "\r\n\r\n"
	idx := strings.Index(raw, separator)
	if idx < 0 {
		return raw
	}

	headers := raw[:idx]
	body := raw[idx:]

	lines := strings.Split(headers, "\r\n")
	var kept []string
	for _, line := range lines {
		if line == "" {
			kept = append(kept, line)
			continue
		}
		colonIdx := strings.Index(line, ":")
		if colonIdx > 0 {
			headerName := strings.TrimSpace(line[:colonIdx])
			if strings.ToLower(headerName) != lowerName {
				kept = append(kept, line)
			}
		} else {
			kept = append(kept, line)
		}
	}

	return strings.Join(kept, "\r\n") + body
}

func replaceBody(raw, newBody string) string {
	separator := "\r\n\r\n"
	idx := strings.Index(raw, separator)
	if idx < 0 {
		return raw + separator + newBody
	}

	headers := raw[:idx]
	result := removeHeader(headers+separator, "content-length")

	if newBody != "" {
		result = httputil.InjectHeader(result[:len(result)-4], "Content-Length", strconv.Itoa(len(newBody))) + separator
		return result + newBody
	}

	return result
}

func editRequestHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, EditRequestInput) (*mcp.CallToolResult, SendRequestOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input EditRequestInput,
	) (*mcp.CallToolResult, SendRequestOutput, error) {
		if input.RequestID == "" {
			return nil, SendRequestOutput{}, fmt.Errorf("requestId is required")
		}

		resp, err := client.Requests.Get(ctx, input.RequestID)
		if err != nil {
			return nil, SendRequestOutput{}, fmt.Errorf("failed to get request: %w", err)
		}

		r := resp.Request
		if r == nil {
			return nil, SendRequestOutput{}, fmt.Errorf("request not found")
		}

		decoded, err := base64.StdEncoding.DecodeString(r.Raw)
		if err != nil {
			return nil, SendRequestOutput{}, fmt.Errorf("failed to decode raw request: %w", err)
		}

		raw := httputil.NormalizeCRLF(string(decoded))

		if input.Method != "" {
			raw = replaceMethod(raw, input.Method)
		}

		if input.Path != "" {
			raw = replacePath(raw, input.Path)
		}

		for name, value := range input.SetHeaders {
			if httputil.HasHeader(raw, name) {
				raw = removeHeader(raw, name)
			}
			raw = httputil.InjectHeader(raw, name, value)
		}

		for _, name := range input.RemoveHeaders {
			raw = removeHeader(raw, name)
		}

		if input.Body != "" {
			raw = replaceBody(raw, input.Body)
		}

		host := r.Host
		if input.Host != "" {
			host = input.Host
		}

		port := r.Port
		if input.Port != 0 {
			port = input.Port
		}

		useTLS := r.IsTls
		if input.TLS != nil {
			useTLS = *input.TLS
		}

		if port == 0 {
			port = httputil.DefaultPort(useTLS)
		}

		sessionID, err := replay.GetOrCreateSession(ctx, client, input.SessionID)
		if err != nil {
			return nil, SendRequestOutput{}, err
		}

		conn := caido.ReplayConnection{Host: host, Port: port, IsTLS: useTLS}
		sendRes, err := replay.Send(
			ctx, client, sessionID, raw, conn, input.SessionID == "",
		)
		if err != nil {
			return nil, SendRequestOutput{}, fmt.Errorf(
				"failed to send request: %w", err,
			)
		}
		sessionID = sendRes.SessionID

		output := SendRequestOutput{SessionID: sessionID}

		entry, pollErr := replay.PollForEntry(
			ctx, client, sessionID, sendRes.PreviousEntryID,
		)
		if pollErr != nil {
			output.Error = fmt.Sprintf(
				"poll failed: %v (use get_replay_entry to retry)", pollErr,
			)
			sess, sErr := client.Replay.GetSession(ctx, sessionID)
			if sErr == nil && sess != nil && sess.ActiveEntryID != "" {
				output.EntryID = sess.ActiveEntryID
			}
			return nil, output, nil
		}

		output.EntryID = entry.ID

		if entry.Request != nil {
			output.RequestID = entry.Request.ID
			output.Request = httputil.ParseBase64(
				entry.Request.Raw, true, false, 0, 0,
			)
			if entry.Request.Response != nil {
				resp := entry.Request.Response
				output.StatusCode = resp.StatusCode
				output.RoundtripMs = resp.RoundtripTime

				bodyLimit := input.BodyLimit
				if bodyLimit == 0 {
					headersOnly := httputil.ParseBase64(resp.Raw, true, false, 0, 0)
					if headersOnly != nil && headersOnly.Fingerprint != nil {
						bodyLimit = httputil.AdaptiveBodyLimit(*headersOnly.Fingerprint, 0)
					} else {
						bodyLimit = httputil.DefaultBodyLimit
					}
				}

				output.Response = httputil.ParseBase64(
					resp.Raw, true, true, input.BodyOffset, bodyLimit,
				)

				if output.Response != nil {
					digest := httputil.ResponseDigest{
						StatusCode: resp.StatusCode,
						BodyHash:   httputil.HashBody([]byte(output.Response.Body)),
						BodySize:   output.Response.BodySize,
					}
					diff := httputil.GlobalResponseCache().GetAndSet(sessionID, digest)
					if diff != nil {
						output.Diff = diff
						if diff.Same {
							output.Response.Body = ""
							output.Response.Headers = nil
						}
					}
				}
			}
		}

		return nil, output, nil
	}
}

func RegisterEditRequestTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_edit_request",
		Description: `Modify and resend an existing request. Fetches original request, applies modifications (method, path, headers, body), preserves auth/cookies, and sends the modified request. Returns same output as send_request.`,
	}, editRequestHandler(client))
}
