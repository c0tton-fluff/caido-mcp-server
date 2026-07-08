package tools

import (
	"context"
	"fmt"
	"net/url"

	"github.com/c0tton-fluff/caido-mcp-server/internal/replay"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GetSessionCookiesInput selects which session jar to inspect.
type GetSessionCookiesInput struct {
	SessionID string `json:"sessionId,omitempty" jsonschema:"Replay session ID. Omit to target the current default session."`
	URL       string `json:"url,omitempty" jsonschema:"URL whose cookies should be returned (RFC 6265 domain/path matching). Required."`
}

// SessionCookie is the redacted shape returned to the LLM. Cookie
// values are redacted by default to avoid leaking auth tokens into
// model context. Use Caido UI for raw values.
type SessionCookie struct {
	Name     string `json:"name"`
	ValueLen int    `json:"valueLen"`
	Domain   string `json:"domain,omitempty"`
	Path     string `json:"path,omitempty"`
	Secure   bool   `json:"secure,omitempty"`
	HTTPOnly bool   `json:"httpOnly,omitempty"`
}

// GetSessionCookiesOutput is the response payload.
type GetSessionCookiesOutput struct {
	SessionID string          `json:"sessionId"`
	URL       string          `json:"url"`
	Cookies   []SessionCookie `json:"cookies"`
	Count     int             `json:"count"`
}

// getSessionCookiesHandler returns RFC 6265-matching cookies for a
// (sessionID, URL) pair. Values are NOT returned, only metadata, to
// avoid leaking tokens into the LLM context.
func getSessionCookiesHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, GetSessionCookiesInput) (*mcp.CallToolResult, GetSessionCookiesOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input GetSessionCookiesInput,
	) (*mcp.CallToolResult, GetSessionCookiesOutput, error) {
		if input.URL == "" {
			return nil, GetSessionCookiesOutput{}, fmt.Errorf(
				"url is required for cookie matching",
			)
		}
		u, err := url.Parse(input.URL)
		if err != nil {
			return nil, GetSessionCookiesOutput{}, fmt.Errorf(
				"invalid url: %w", err,
			)
		}

		sessionID, err := replay.GetOrCreateSession(
			ctx, client, input.SessionID,
		)
		if err != nil {
			return nil, GetSessionCookiesOutput{}, err
		}

		cookies := replay.DefaultCookieStore().Cookies(sessionID, u)
		out := GetSessionCookiesOutput{
			SessionID: sessionID,
			URL:       input.URL,
			Cookies:   make([]SessionCookie, 0, len(cookies)),
			Count:     len(cookies),
		}
		for _, c := range cookies {
			if c == nil {
				continue
			}
			out.Cookies = append(out.Cookies, SessionCookie{
				Name:     c.Name,
				ValueLen: len(c.Value),
				Domain:   c.Domain,
				Path:     c.Path,
				Secure:   c.Secure,
				HTTPOnly: c.HttpOnly,
			})
		}
		return nil, out, nil
	}
}

// RegisterGetSessionCookiesTool registers caido_get_session_cookies
// with the MCP server.
func RegisterGetSessionCookiesTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_get_session_cookies",
		Description: `List cookies stored in the session jar that match a URL (RFC 6265). Returns metadata only (name/length/domain/path/flags); values are not exposed to avoid leaking tokens into model context. Use to debug cookie persistence between send_request calls.`,
		Annotations: readOnlyAnn(),
	}, getSessionCookiesHandler(client))
}
