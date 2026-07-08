package tools

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"

	"github.com/c0tton-fluff/caido-mcp-server/internal/httputil"
	"github.com/c0tton-fluff/caido-mcp-server/internal/replay"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// SendRequestInput is the input for the send_request tool
type SendRequestInput struct {
	Raw          string `json:"raw" jsonschema:"required,Raw HTTP request including headers and body"`
	Host         string `json:"host,omitempty" jsonschema:"Target host (overrides Host header)"`
	Port         int    `json:"port,omitempty" jsonschema:"Target port (default based on TLS)"`
	TLS          *bool  `json:"tls,omitempty" jsonschema:"Use HTTPS (default true)"`
	SessionID    string `json:"sessionId,omitempty" jsonschema:"Replay session ID (optional)"`
	BodyLimit    int    `json:"bodyLimit,omitempty" jsonschema:"Response body byte limit (default 2000)"`
	BodyOffset   int    `json:"bodyOffset,omitempty" jsonschema:"Response body byte offset (default 0)"`
	UseCookieJar *bool  `json:"useCookieJar,omitempty" jsonschema:"Auto-inject session cookies and persist Set-Cookie (default true). Set false to disable for this call only."`
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
	Diff        *httputil.DiffResult    `json:"diff,omitempty"`
	CookieJar   *CookieJarStatus        `json:"cookieJar,omitempty"`
	Error       string                  `json:"error,omitempty"`
}

// CookieJarStatus reports cookie-jar activity for a single send.
type CookieJarStatus struct {
	Enabled         bool     `json:"enabled"`
	InjectedCookies []string `json:"injectedCookies,omitempty"`
	StoredCookies   []string `json:"storedCookies,omitempty"`
	Skipped         string   `json:"skipped,omitempty"`
}

// buildRequestURL synthesizes the *url.URL targeted by raw, used for
// RFC 6265 cookie matching against the session jar.
func buildRequestURL(host string, port int, useTLS bool, raw string) *url.URL {
	scheme := "http"
	if useTLS {
		scheme = "https"
	}
	defaultPort := httputil.DefaultPort(useTLS)
	hostHeader := host
	if port != 0 && port != defaultPort {
		hostHeader = fmt.Sprintf("%s:%d", host, port)
	}
	target := httputil.ExtractPath(raw)
	u, err := url.Parse(scheme + "://" + hostHeader + target)
	if err != nil {
		return &url.URL{Scheme: scheme, Host: hostHeader, Path: "/"}
	}
	return u
}

// cookiesToNames extracts the Name field from each cookie for output.
// nil cookies and unnamed entries are skipped.
func cookiesToNames(cookies []*http.Cookie) []string {
	names := make([]string, 0, len(cookies))
	for _, c := range cookies {
		if c != nil && c.Name != "" {
			names = append(names, c.Name)
		}
	}
	return names
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
		if err := checkRawSize("raw", input.Raw); err != nil {
			return nil, SendRequestOutput{}, err
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
			port = httputil.DefaultPort(useTLS)
		}

		sessionID, err := replay.GetOrCreateSession(
			ctx, client, input.SessionID,
		)
		if err != nil {
			return nil, SendRequestOutput{}, err
		}

		// Cookie jar (RFC 6265): inject session cookies into raw when
		// the user did not already set a Cookie header. Default ON.
		useJar := true
		if input.UseCookieJar != nil {
			useJar = *input.UseCookieJar
		}
		jarStatus := &CookieJarStatus{Enabled: useJar}
		reqURL := buildRequestURL(host, port, useTLS, raw)

		if useJar {
			if httputil.HasHeader(raw, "Cookie") {
				jarStatus.Skipped = "explicit Cookie header preserved"
			} else {
				cookies := replay.DefaultCookieStore().Cookies(sessionID, reqURL)
				if len(cookies) > 0 {
					raw = httputil.InjectHeader(
						raw, "Cookie", httputil.BuildCookieHeader(cookies),
					)
					jarStatus.InjectedCookies = cookiesToNames(cookies)
				}
			}
		}

		// Send via the 0.57 draft-then-start flow. When the default
		// session was used, allow Send to replace the cached session on a
		// busy/empty-session fallback.
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

		output := SendRequestOutput{
			SessionID: sessionID,
			CookieJar: jarStatus,
		}

		entry, pollErr := replay.PollForEntry(
			ctx, client, sessionID, sendRes.PreviousEntryID,
		)
		if pollErr != nil {
			output.Error = fmt.Sprintf(
				"poll failed: %v (use get_replay_entry to retry)",
				pollErr,
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
					resp.Raw, true, true,
					input.BodyOffset, bodyLimit,
				)

				// Diff against previous response in same session.
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

				// Persist Set-Cookie back into the session jar.
				if useJar {
					decoded, decErr := base64.StdEncoding.DecodeString(resp.Raw)
					if decErr == nil {
						setCookies := httputil.ExtractRawSetCookies(decoded)
						if len(setCookies) > 0 {
							storeErr := replay.DefaultCookieStore().SetCookies(
								sessionID, reqURL, setCookies,
							)
							if storeErr == nil {
								jarStatus.StoredCookies = cookiesToNames(setCookies)
							}
						}
					}
				}
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
		Description: `Send HTTP request and return response inline. Returns statusCode, headers, body. Polls up to 10s for response. On timeout, returns entryId for follow-up via get_replay_entry. Session cookies (Set-Cookie) auto-persist between calls sharing the same sessionId; pass useCookieJar:false to disable for a single call.`,
		Annotations: writeAnn(false, false, true),
	}, sendRequestHandler(client))
}
