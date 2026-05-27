package tools

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"strconv"

	"github.com/c0tton-fluff/caido-mcp-server/internal/httputil"
	caido "github.com/caido-community/sdk-go"
	gen "github.com/caido-community/sdk-go/graphql"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// CreateAutomateSessionInput is the input for the create_automate_session tool
type CreateAutomateSessionInput struct {
	RequestID string `json:"requestId,omitempty" jsonschema:"Existing request ID to seed the fuzzing session with"`
	Raw       string `json:"raw,omitempty" jsonschema:"Raw HTTP request to seed the session (alternative to requestId)"`
	Host      string `json:"host,omitempty" jsonschema:"Target host (required when using raw, overrides Host header)"`
	Port      int    `json:"port,omitempty" jsonschema:"Target port (default based on TLS)"`
	TLS       *bool  `json:"tls,omitempty" jsonschema:"Use HTTPS (default true, only for raw)"`
}

// CreateAutomateSessionOutput is the output of the create_automate_session tool
type CreateAutomateSessionOutput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func createAutomateSessionHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, CreateAutomateSessionInput) (*mcp.CallToolResult, CreateAutomateSessionOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input CreateAutomateSessionInput,
	) (*mcp.CallToolResult, CreateAutomateSessionOutput, error) {
		if input.RequestID == "" && input.Raw == "" {
			return nil, CreateAutomateSessionOutput{}, fmt.Errorf(
				"either requestId or raw is required to seed the session",
			)
		}

		source := &gen.RequestSourceInput{}
		switch {
		case input.RequestID != "":
			source.Id = &input.RequestID
		default:
			if len(input.Raw) > 1048576 {
				return nil, CreateAutomateSessionOutput{}, fmt.Errorf(
					"raw request exceeds max length of 1MB",
				)
			}
			raw := httputil.NormalizeCRLF(input.Raw)

			host := input.Host
			if host == "" {
				host = httputil.ParseHostHeader(input.Raw)
			}
			if host == "" {
				return nil, CreateAutomateSessionOutput{}, fmt.Errorf(
					"host is required when using raw (provide host or a Host header)",
				)
			}
			if h, p, err := net.SplitHostPort(host); err == nil {
				host = h
				if input.Port == 0 {
					if port, pErr := strconv.Atoi(p); pErr == nil {
						input.Port = port
					}
				}
			}

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

			source.Raw = &gen.RequestRawInput{
				ConnectionInfo: gen.ConnectionInfoInput{
					Host:  host,
					Port:  port,
					IsTLS: useTLS,
				},
				Raw: base64.StdEncoding.EncodeToString([]byte(raw)),
			}
		}

		resp, err := client.Automate.CreateSession(ctx, &gen.CreateAutomateSessionInput{
			RequestSource: source,
		})
		if err != nil {
			return nil, CreateAutomateSessionOutput{}, fmt.Errorf("create automate session: %w", err)
		}

		session := resp.CreateAutomateSession.Session
		if session == nil {
			return nil, CreateAutomateSessionOutput{}, fmt.Errorf("create automate session returned nil")
		}

		return nil, CreateAutomateSessionOutput{
			ID:   session.Id,
			Name: session.Name,
		}, nil
	}
}

func RegisterCreateAutomateSessionTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "caido_create_automate_session",
		Description: `Create an Automate (fuzzing) session seeded from an existing ` +
			`request (requestId) or a raw HTTP request (raw + host). Returns the ` +
			`new session id. Configure payload placeholders in the Caido UI, then ` +
			`start fuzzing with caido_automate_task_control.`,
	}, createAutomateSessionHandler(client))
}
