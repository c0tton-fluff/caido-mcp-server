package tools

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/httputil"
	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/replay"
	caido "github.com/caido-community/sdk-go"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// WebSocket replay tools: open a socket through Caido from a WS replay
// session, send frames, long-poll pushed frames, close. Every call is keyed
// by the replay session ID so the agent juggles one handle, and the traffic
// lands in Caido's WS history and the session's replay tab.

const (
	defaultWsConnectTimeoutS = 10
	maxWsConnectTimeoutS     = 60
	defaultWsCloseTimeoutS   = 10
	maxWsCloseTimeoutS       = 60
	defaultWsHandshakeBody   = 1024
	maxWsHandshakeBody       = 8192
	maxWsFrames              = 50
	maxWsDelayMs             = 10000
	defaultWsReadWaitMs      = 3000
	maxWsReadWaitMs          = 120000
	defaultWsReadLimit       = 50
	maxWsReadLimit           = 200
	defaultWsReadBody        = 4096
	maxWsReadBody            = 65536
	maxWsHeaders             = 50
)

// --- caido_ws_connect ---

// WsConnectInput is the input for the ws_connect tool.
type WsConnectInput struct {
	URL          string            `json:"url,omitempty" jsonschema:"WebSocket URL (ws://, wss://, http:// or https://). The upgrade request is built from it plus headers and subprotocols"`
	Headers      map[string]string `json:"headers,omitempty" jsonschema:"Extra handshake headers, e.g. {\"Origin\":\"https://app.example\",\"Cookie\":\"sid=...\"}. Override the generated ones by name"`
	Subprotocols []string          `json:"subprotocols,omitempty" jsonschema:"Sec-WebSocket-Protocol values, e.g. [\"graphql-ws\"]"`
	Raw          string            `json:"raw,omitempty" jsonschema:"Full raw upgrade request instead of url (needs host)"`
	Host         string            `json:"host,omitempty" jsonschema:"Target host for raw (no scheme)"`
	Port         int               `json:"port,omitempty" jsonschema:"Target port for raw (default 443 with TLS, 80 without)"`
	TLS          *bool             `json:"tls,omitempty" jsonschema:"Use TLS for raw (default true)"`
	RequestID    string            `json:"requestId,omitempty" jsonschema:"History request ID of a captured upgrade request to replay instead of url/raw"`
	SessionID    string            `json:"sessionId,omitempty" jsonschema:"Reuse this WS replay session: its handshake is replaced and an open connection on it is closed first. Omit to create a session"`
	CollectionID string            `json:"collectionId,omitempty" jsonschema:"File the session in this replay collection"`
	Name         string            `json:"name,omitempty" jsonschema:"Tab name for the session"`
	Reuse        bool              `json:"reuse,omitempty" jsonschema:"When the Caido plan's WebSocket session limit blocks creating a session and exactly one WS session exists, reuse it: moved into collectionId, renamed to name, open connection closed. Its earlier run stays in WS history (previousStreamId)"`
	Connect      *bool             `json:"connect,omitempty" jsonschema:"Open the socket (default true). false only prepares the tab for a human to click Connect"`
	TimeoutSec   int               `json:"timeoutSec,omitempty" jsonschema:"Seconds to wait for the handshake response, and on a reused session for Caido to close its open connection (default 10, max 60)"`
	BodyLimit    int               `json:"bodyLimit,omitempty" jsonschema:"Byte limit for a rejected handshake's response body (default 1024, max 8192)"`
}

// WsConnectOutput is the output of the ws_connect tool.
type WsConnectOutput struct {
	SessionID string `json:"sessionId"`
	Created   bool   `json:"created"`
	// Reused is true when reuse=true took the only WS session because the
	// plan's limit blocked creating one.
	Reused         bool `json:"reused,omitempty"`
	ClosedPrevious bool `json:"closedPrevious,omitempty"`
	// PreviousStreamID is the earlier run a reused session's tab no longer
	// shows; read it with caido_ws_read streamId.
	PreviousStreamID string `json:"previousStreamId,omitempty"`
	// MovedFrom is the collection a reused session was moved out of.
	MovedFrom      string `json:"movedFrom,omitempty"`
	Name           string `json:"name,omitempty"`
	CollectionID   string `json:"collectionId,omitempty"`
	CollectionName string `json:"collectionName,omitempty"`
	// Open is true while the socket is connected; pass sessionId to
	// caido_ws_send / caido_ws_read / caido_ws_close.
	Open     bool   `json:"open"`
	StreamID string `json:"streamId,omitempty"`
	// Pending is true when timeoutSec passed before any handshake response.
	Pending         bool              `json:"pending,omitempty"`
	StatusCode      int               `json:"statusCode,omitempty"`
	ResponseHeaders []httputil.Header `json:"responseHeaders,omitempty"`
	Body            string            `json:"body,omitempty"`
	Error           string            `json:"error,omitempty"`
}

// wsHeader is one handshake header, in send order.
type wsHeader struct{ name, value string }

// buildWsHandshake builds a plain-text upgrade request for rawURL.
func buildWsHandshake(
	rawURL string, headers map[string]string, protos []string,
) (string, caido.ReplayConnection, error) {
	u, conn, err := parseWsURL(rawURL)
	if err != nil {
		return "", conn, err
	}
	hs, err := wsHandshakeHeaders(u.Host, headers, protos)
	if err != nil {
		return "", conn, err
	}
	target := cmp.Or(u.RequestURI(), "/")
	var b strings.Builder
	b.WriteString("GET " + target + " HTTP/1.1\r\n")
	for _, h := range hs {
		b.WriteString(h.name + ": " + h.value + "\r\n")
	}
	b.WriteString("\r\n")
	return b.String(), conn, nil
}

// parseWsURL parses a ws, wss, http or https URL and derives where to
// connect.
func parseWsURL(rawURL string) (*url.URL, caido.ReplayConnection, error) {
	var conn caido.ReplayConnection
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, conn, fmt.Errorf("parse url: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "ws", "http":
	case "wss", "https":
		conn.IsTLS = true
	default:
		return nil, conn, fmt.Errorf("url scheme must be ws, wss, http or https, got %q", u.Scheme)
	}
	conn.Host = u.Hostname()
	if conn.Host == "" {
		return nil, conn, fmt.Errorf("url has no host")
	}
	conn.Port = httputil.DefaultPort(conn.IsTLS)
	if p := u.Port(); p != "" {
		port, err := strconv.Atoi(p)
		if err != nil || port < 1 || port > 65535 {
			return nil, conn, fmt.Errorf("invalid port %q", p)
		}
		conn.Port = port
	}
	return u, conn, nil
}

// wsHandshakeHeaders returns the upgrade headers for host: the generated
// ones, then the user's in name order, each replacing a generated header
// of the same name.
func wsHandshakeHeaders(host string, headers map[string]string, protos []string) ([]wsHeader, error) {
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate Sec-WebSocket-Key: %w", err)
	}
	hs := []wsHeader{
		{"Host", host},
		{"Upgrade", "websocket"},
		{"Connection", "Upgrade"},
		{"Sec-WebSocket-Version", "13"},
		{"Sec-WebSocket-Key", base64.StdEncoding.EncodeToString(key)},
	}
	for _, p := range protos {
		if !isHTTPToken(p) {
			return nil, fmt.Errorf("invalid subprotocol %q (must be an http token)", p)
		}
	}
	if len(protos) > 0 {
		hs = append(hs, wsHeader{"Sec-WebSocket-Protocol", strings.Join(protos, ", ")})
	}
	names := slices.Sorted(maps.Keys(headers))
	for i, n := range names {
		v := headers[n]
		if !isHTTPToken(n) || strings.ContainsAny(v, "\r\n\x00") {
			return nil, fmt.Errorf("invalid header %q", n)
		}
		// Keys differing only in case would silently overwrite each other.
		if slices.ContainsFunc(names[:i], func(p string) bool { return strings.EqualFold(p, n) }) {
			return nil, fmt.Errorf("header %q given twice (names are case-insensitive)", n)
		}
		if j := slices.IndexFunc(hs, func(h wsHeader) bool { return strings.EqualFold(h.name, n) }); j >= 0 {
			hs[j] = wsHeader{n, v}
		} else {
			hs = append(hs, wsHeader{n, v})
		}
	}
	return hs, nil
}

// isHTTPToken reports whether s is an RFC 9110 token: what header names
// and Sec-WebSocket-Protocol values must be.
func isHTTPToken(s string) bool {
	if s == "" {
		return false
	}
	for i := range len(s) {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case strings.IndexByte("!#$%&'*+-.^_`|~", c) >= 0:
		default:
			return false
		}
	}
	return true
}

// rawHandshake validates a raw upgrade request and its target.
func rawHandshake(in WsConnectInput) (string, caido.ReplayConnection, error) {
	var none caido.ReplayConnection
	if in.Host == "" || strings.ContainsAny(in.Host, "/ ") {
		return "", none, fmt.Errorf("host is required with raw (bare host, no scheme or path)")
	}
	if in.Port < 0 || in.Port > 65535 {
		return "", none, fmt.Errorf("port must be 1..65535")
	}
	if err := checkRawSize("raw", in.Raw); err != nil {
		return "", none, err
	}
	useTLS := true
	if in.TLS != nil {
		useTLS = *in.TLS
	}
	port := cmp.Or(in.Port, httputil.DefaultPort(useTLS))
	return httputil.NormalizeCRLF(in.Raw),
		caido.ReplayConnection{Host: in.Host, Port: port, IsTLS: useTLS}, nil
}

// security: any host is a valid target and a raw handshake is sent as
// given: this is a proxy pentest tool, the traffic goes through the user's
// Caido, and caido_send_request allows the same. Headers built from url
// input are validated (token names, no CR/LF/NUL in values) so a value
// cannot smuggle extra headers or a second request into the handshake.

// wsHandshakeSource resolves the one handshake source the input names.
func wsHandshakeSource(
	ctx context.Context, client *caido.Client, in WsConnectInput,
) (string, caido.ReplayConnection, error) {
	var none caido.ReplayConnection
	n := 0
	for _, set := range []bool{in.URL != "", in.Raw != "", in.RequestID != ""} {
		if set {
			n++
		}
	}
	if n != 1 {
		return "", none, fmt.Errorf("give exactly one of url, raw or requestId")
	}
	if len(in.Headers) > maxWsHeaders {
		return "", none, fmt.Errorf("headers exceeds %d entries", maxWsHeaders)
	}
	if in.URL == "" && (len(in.Headers) > 0 || len(in.Subprotocols) > 0) {
		return "", none, fmt.Errorf("headers and subprotocols apply only with url; edit raw instead")
	}
	switch {
	case in.URL != "":
		return buildWsHandshake(in.URL, in.Headers, in.Subprotocols)
	case in.Raw != "":
		return rawHandshake(in)
	default:
		return replay.HandshakeFromRequest(ctx, client, in.RequestID)
	}
}

func wsConnectHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, WsConnectInput) (*mcp.CallToolResult, WsConnectOutput, error) {
	return func(
		ctx context.Context, req *mcp.CallToolRequest, in WsConnectInput,
	) (*mcp.CallToolResult, WsConnectOutput, error) {
		raw, conn, err := wsHandshakeSource(ctx, client, in)
		if err != nil {
			return nil, WsConnectOutput{}, err
		}
		timeout := clampLimit(in.TimeoutSec, defaultWsConnectTimeoutS, maxWsConnectTimeoutS)
		bodyLimit := clampLimit(in.BodyLimit, defaultWsHandshakeBody, maxWsHandshakeBody)
		skipOpen := in.Connect != nil && !*in.Connect

		c, err := replay.WSConnect(ctx, client, replay.WSConnectOptions{
			SessionID:    in.SessionID,
			CollectionID: in.CollectionID,
			Name:         in.Name,
			Raw:          raw,
			Conn:         conn,
			SkipOpen:     skipOpen,
			Timeout:      time.Duration(timeout) * time.Second,
			ReuseOnLimit: in.Reuse,
		})
		if err != nil {
			if c != nil && c.SessionID != "" {
				err = fmt.Errorf("session %s: %w", c.SessionID, err)
				if c.ClosedPrevious {
					err = fmt.Errorf("%w (its previous connection was closed)", err)
				}
				if c.PreviousStreamID != "" {
					err = fmt.Errorf("%w; the earlier run is stream %s (caido_ws_read streamId)",
						err, c.PreviousStreamID)
				}
			}
			return nil, WsConnectOutput{}, err
		}
		out := WsConnectOutput{
			SessionID:        c.SessionID,
			Created:          c.Created,
			Reused:           c.Reused,
			ClosedPrevious:   c.ClosedPrevious,
			PreviousStreamID: c.PreviousStreamID,
			MovedFrom:        c.MovedFrom,
			Name:             c.Name,
			CollectionID:     c.CollectionID,
			CollectionName:   c.CollectionName,
			Open:             c.Open,
			StreamID:         c.StreamID,
			StatusCode:       c.StatusCode,
			Error:            c.Error,
		}
		if skipOpen {
			return nil, out, nil
		}
		out.Pending = c.StatusCode == 0 && c.Error == ""
		if msg := httputil.ParseBase64(c.ResponseRaw, true, c.StatusCode != http.StatusSwitchingProtocols, 0, bodyLimit); msg != nil {
			out.ResponseHeaders = msg.Headers
			out.Body = msg.Body
		}
		return nil, out, nil
	}
}

// --- caido_ws_send ---

// WsFrameInput is one frame to send.
type WsFrameInput struct {
	Body   string `json:"body" jsonschema:"Frame payload"`
	Format string `json:"format,omitempty" jsonschema:"TEXT (default) or BINARY"`
	Base64 bool   `json:"base64,omitempty" jsonschema:"body is base64 (for BINARY payloads)"`
}

// WsSendInput is the input for the ws_send tool.
type WsSendInput struct {
	SessionID string         `json:"sessionId" jsonschema:"required,WS replay session ID from caido_ws_connect"`
	Frames    []WsFrameInput `json:"frames" jsonschema:"required,Frames to send in order (1..50)"`
	DelayMs   int            `json:"delayMs,omitempty" jsonschema:"Pause between frames in ms (max 10000)"`
}

// WsSendOutput is the output of the ws_send tool.
type WsSendOutput struct {
	Sent       int      `json:"sent"`
	MessageIDs []string `json:"messageIds"`
}

// parseWsFrames validates and decodes the frames to send.
func parseWsFrames(in []WsFrameInput) ([]replay.WSFrame, error) {
	frames := make([]replay.WSFrame, len(in))
	for i, f := range in {
		format := cmp.Or(strings.ToUpper(f.Format), "TEXT")
		if format != "TEXT" && format != "BINARY" {
			return nil, fmt.Errorf("frames[%d]: format must be TEXT or BINARY", i)
		}
		data := []byte(f.Body)
		if f.Base64 {
			d, err := base64.StdEncoding.DecodeString(f.Body)
			if err != nil {
				return nil, fmt.Errorf("frames[%d]: invalid base64: %w", i, err)
			}
			data = d
		}
		if len(data) > maxRawRequestBytes {
			return nil, fmt.Errorf("frames[%d]: body exceeds 1MB limit", i)
		}
		frames[i] = replay.WSFrame{Format: format, Data: data}
	}
	return frames, nil
}

func wsSendHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, WsSendInput) (*mcp.CallToolResult, WsSendOutput, error) {
	return func(
		ctx context.Context, req *mcp.CallToolRequest, in WsSendInput,
	) (*mcp.CallToolResult, WsSendOutput, error) {
		if in.SessionID == "" {
			return nil, WsSendOutput{}, fmt.Errorf("sessionId is required")
		}
		if n := len(in.Frames); n == 0 || n > maxWsFrames {
			return nil, WsSendOutput{}, fmt.Errorf("frames must contain 1..%d items, got %d", maxWsFrames, n)
		}
		if in.DelayMs < 0 || in.DelayMs > maxWsDelayMs {
			return nil, WsSendOutput{}, fmt.Errorf("delayMs must be 0..%d", maxWsDelayMs)
		}
		frames, err := parseWsFrames(in.Frames)
		if err != nil {
			return nil, WsSendOutput{}, err
		}
		state, err := replay.WSSessionState(ctx, client, in.SessionID)
		if err != nil {
			return nil, WsSendOutput{}, err
		}
		if state.TaskID == "" {
			return nil, WsSendOutput{}, fmt.Errorf(
				"session %s: %w; open one with caido_ws_connect", in.SessionID, replay.ErrWSNotOpen,
			)
		}
		n, ids, err := replay.WSSend(ctx, client, state.TaskID, frames,
			time.Duration(in.DelayMs)*time.Millisecond)
		if err != nil {
			// A tool error carries no structured output, so name the stored
			// messages of the frames that did go out in the text.
			return nil, WsSendOutput{}, fmt.Errorf(
				"sent %d of %d frames (message ids %v): %w", n, len(frames), ids, err,
			)
		}
		return nil, WsSendOutput{Sent: n, MessageIDs: ids}, nil
	}
}

// --- caido_ws_read ---

// WsReadInput is the input for the ws_read tool.
type WsReadInput struct {
	SessionID     string   `json:"sessionId,omitempty" jsonschema:"WS replay session ID from caido_ws_connect (or give streamId)"`
	StreamID      string   `json:"streamId,omitempty" jsonschema:"Read any WS stream directly, e.g. one from caido_list_ws_streams (no waiting on connection state)"`
	After         string   `json:"after,omitempty" jsonschema:"Cursor from the previous caido_ws_read; omit to read from the first frame"`
	WaitMs        int      `json:"waitMs,omitempty" jsonschema:"Long-poll up to this many ms for a frame meeting the stop rule (default 3000, max 120000)"`
	UntilContains string   `json:"untilContains,omitempty" jsonschema:"Stop once a returned frame contains this text"`
	UntilCount    int      `json:"untilCount,omitempty" jsonschema:"Stop once this many frames are returned (not with untilContains). Without either, the read returns every frame available as soon as any arrive"`
	SkipBodies    []string `json:"skipBodies,omitempty" jsonschema:"Drop frames whose body equals one of these exactly, e.g. keep-alives [\"{\\\"type\\\":\\\"ka\\\"}\"]"`
	IncludeSent   bool     `json:"includeSent,omitempty" jsonschema:"Also return frames the client sent (default: only frames from the server)"`
	Limit         int      `json:"limit,omitempty" jsonschema:"Max frames to return (default 50, max 200); read again with cursor for more"`
	BodyLimit     int      `json:"bodyLimit,omitempty" jsonschema:"Max bytes of each frame body (default 4096, max 65536)"`
}

// WsFrame is one frame in ws_read output.
type WsFrame struct {
	ID string `json:"id"`
	// Direction is sent (client to server) or received (server to client).
	Direction string `json:"direction"`
	Format    string `json:"format"`
	Length    int    `json:"length"`
	Body      string `json:"body"`
	Encoding  string `json:"encoding,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

// WsReadOutput is the output of the ws_read tool.
type WsReadOutput struct {
	Frames []WsFrame `json:"frames"`
	// Cursor resumes after everything scanned; pass it as after.
	Cursor string `json:"cursor,omitempty"`
	// Matched reports the until rule was met (or, without one, that a frame
	// arrived) before waitMs passed or the limit or scan cap was hit.
	Matched bool `json:"matched"`
	// Open reports whether the session's connection is still open; absent
	// when reading by streamId.
	Open *bool `json:"open,omitempty"`
	// SkippedSent counts client frames left out (includeSent off),
	// SkippedBodies frames matching skipBodies, SkippedControl PING/PONG.
	SkippedSent    int `json:"skippedSent,omitempty"`
	SkippedBodies  int `json:"skippedBodies,omitempty"`
	SkippedControl int `json:"skippedControl,omitempty"`
	// More is true when the read stopped at its scan cap with older frames
	// still unread; read again with cursor.
	More bool `json:"more,omitempty"`
}

// security: frame bodies are returned unredacted, as caido_list_ws_messages
// returns them; includeSent is off by default so the agent's own auth frames
// (e.g. a connection_init token) are not echoed back unless asked for.
// Handshake response headers go through httputil's sensitive-header
// redaction.

// wsFrameBody renders a frame body: text as is, a close frame as its status
// code and reason, anything else (or invalid UTF-8) as base64.
func wsFrameBody(m replay.WSMessage, limit int) (body, encoding string, truncated bool) {
	data := m.Data
	if m.Format == "CLOSE" && len(data) >= 2 {
		code := binary.BigEndian.Uint16(data[:2])
		reason := strings.ToValidUTF8(string(data[2:]), "\uFFFD")
		return strings.TrimSpace(fmt.Sprintf("%d %s", code, reason)), "", false
	}
	truncated = m.Truncated
	if len(data) > limit {
		data, truncated = data[:limit], true
	}
	if truncated {
		// Drop a rune the cut split in two, nothing else.
		for i := len(data) - 1; i >= 0 && i >= len(data)-utf8.UTFMax; i-- {
			if utf8.RuneStart(data[i]) {
				if !utf8.FullRune(data[i:]) {
					data = data[:i]
				}
				break
			}
		}
	}
	if (m.Format == "TEXT" || m.Format == "CLOSE") && utf8.Valid(data) {
		return string(data), "", truncated
	}
	return base64.StdEncoding.EncodeToString(data), "base64", truncated
}

// sessionStream resolves a session to the stream to read and whether its
// connection is open. For an open one it sets opts.Alive so the read
// notices a close; for a closed one it drops the wait, as nothing more
// will arrive.
func sessionStream(
	ctx context.Context, client *caido.Client, sessionID string, opts *replay.WSReadOptions,
) (string, *bool, error) {
	state, err := replay.WSSessionState(ctx, client, sessionID)
	if err != nil {
		return "", nil, err
	}
	isOpen := state.TaskID != ""
	switch {
	case state.StreamID == "" && isOpen:
		return "", nil, fmt.Errorf("session %s: handshake still pending, read again shortly", sessionID)
	case state.StreamID == "":
		return "", nil, fmt.Errorf("session %s has never connected; use caido_ws_connect", sessionID)
	}
	if !isOpen {
		opts.Wait = 0
		return state.StreamID, &isOpen, nil
	}
	opts.Alive = func(ctx context.Context) (bool, error) {
		task, err := replay.WSOpenTask(ctx, client, sessionID)
		return task == state.TaskID, err
	}
	return state.StreamID, &isOpen, nil
}

func wsReadHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, WsReadInput) (*mcp.CallToolResult, WsReadOutput, error) {
	return func(
		ctx context.Context, req *mcp.CallToolRequest, in WsReadInput,
	) (*mcp.CallToolResult, WsReadOutput, error) {
		if (in.SessionID == "") == (in.StreamID == "") {
			return nil, WsReadOutput{}, fmt.Errorf("give exactly one of sessionId or streamId")
		}
		if in.UntilCount < 0 {
			return nil, WsReadOutput{}, fmt.Errorf("untilCount must be >= 0")
		}
		if in.UntilCount > 0 && in.UntilContains != "" {
			return nil, WsReadOutput{}, fmt.Errorf("give at most one of untilContains and untilCount")
		}
		wait := clampLimit(in.WaitMs, defaultWsReadWaitMs, maxWsReadWaitMs)
		bodyLimit := clampLimit(in.BodyLimit, defaultWsReadBody, maxWsReadBody)
		skip := make(map[string]bool, len(in.SkipBodies))
		for _, b := range in.SkipBodies {
			skip[b] = true
		}
		opts := replay.WSReadOptions{
			After:         in.After,
			Wait:          time.Duration(wait) * time.Millisecond,
			UntilContains: in.UntilContains,
			UntilCount:    in.UntilCount,
			SkipBodies:    skip,
			IncludeSent:   in.IncludeSent,
			Limit:         clampLimit(in.Limit, defaultWsReadLimit, maxWsReadLimit),
			MaxBody:       bodyLimit,
		}

		streamID := in.StreamID
		var open *bool
		if in.SessionID != "" {
			var err error
			streamID, open, err = sessionStream(ctx, client, in.SessionID, &opts)
			if err != nil {
				return nil, WsReadOutput{}, err
			}
		}

		res, err := replay.WSRead(ctx, client, streamID, opts)
		if err != nil {
			return nil, WsReadOutput{}, err
		}
		if res.Closed {
			closed := false
			open = &closed
		}
		out := WsReadOutput{
			Frames:         make([]WsFrame, 0, len(res.Messages)),
			Cursor:         res.Cursor,
			Matched:        res.Matched,
			Open:           open,
			SkippedSent:    res.SkippedSent,
			SkippedBodies:  res.SkippedBodies,
			SkippedControl: res.SkippedControl,
			More:           res.More,
		}
		for _, m := range res.Messages {
			body, enc, trunc := wsFrameBody(m, bodyLimit)
			dir := "received"
			if m.Sent {
				dir = "sent"
			}
			out.Frames = append(out.Frames, WsFrame{
				ID: m.ID, Direction: dir, Format: m.Format, Length: m.Length,
				Body: body, Encoding: enc, Truncated: trunc,
			})
		}
		return nil, out, nil
	}
}

// --- caido_ws_close ---

// WsCloseInput is the input for the ws_close tool.
type WsCloseInput struct {
	SessionID  string `json:"sessionId" jsonschema:"required,WS replay session ID from caido_ws_connect"`
	TimeoutSec int    `json:"timeoutSec,omitempty" jsonschema:"Seconds to wait for Caido to close the socket (default 10, max 60)"`
}

// WsCloseOutput is the output of the ws_close tool.
type WsCloseOutput struct {
	SessionID string `json:"sessionId"`
	// Closed is false when no connection was open or the close timed out.
	Closed bool `json:"closed"`
	// TimedOut is true when Caido did not confirm the close within
	// timeoutSec; the connection may still be open.
	TimedOut bool `json:"timedOut,omitempty"`
}

func wsCloseHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, WsCloseInput) (*mcp.CallToolResult, WsCloseOutput, error) {
	return func(
		ctx context.Context, req *mcp.CallToolRequest, in WsCloseInput,
	) (*mcp.CallToolResult, WsCloseOutput, error) {
		if in.SessionID == "" {
			return nil, WsCloseOutput{}, fmt.Errorf("sessionId is required")
		}
		timeout := clampLimit(in.TimeoutSec, defaultWsCloseTimeoutS, maxWsCloseTimeoutS)
		res, err := replay.WSCloseSession(ctx, client, in.SessionID, time.Duration(timeout)*time.Second)
		if err != nil {
			return nil, WsCloseOutput{}, err
		}
		return nil, WsCloseOutput{SessionID: in.SessionID, Closed: res.Closed, TimedOut: res.TimedOut}, nil
	}
}

// --- registration ---

// RegisterWsConnectTool registers caido_ws_connect.
func RegisterWsConnectTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:  "caido_ws_connect",
		Title: "WebSocket Replay: Connect",
		Description: `Open a WebSocket through Caido from a WS replay session (Caido 0.59+), so the traffic lands in WS history and a named replay tab. ` +
			`Give exactly one handshake source: url (+ headers, subprotocols), raw (+ host/port/tls), or requestId of a captured upgrade request. ` +
			`Returns sessionId (the handle for caido_ws_send / caido_ws_read / caido_ws_close), open, statusCode (101 on success), response headers, and the name and collectionId the tab is filed under; a rejected handshake also returns its body. ` +
			`Some Caido plans allow a single WS session: pass reuse=true (moves it into collectionId, renames it to name) or its sessionId to reuse it with a new handshake (an open connection on it is closed first). ` +
			`Reusing replaces what the tab shows: the earlier run stays in WS history as previousStreamId, readable with caido_ws_read streamId, so cite that stream as evidence before a control run. ` +
			`connect=false only prepares the tab.`,
		InputSchema: schemaFor[WsConnectInput](func(s *jsonschema.Schema) {
			if p := prop(s, "raw"); p != nil {
				p.MaxLength = intPtr(maxRawRequestBytes)
			}
		}),
		// Destructive: with sessionId it closes a live connection and
		// replaces the stored handshake.
		Annotations: writeAnn(true, false, true),
	}, wsConnectHandler(client))
}

// RegisterWsSendTool registers caido_ws_send.
func RegisterWsSendTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:  "caido_ws_send",
		Title: "WebSocket Replay: Send Frames",
		Description: `Send frames in order on a session's open WebSocket (from caido_ws_connect), optionally delayMs apart. ` +
			`Returns the stored message IDs. Read replies with caido_ws_read.`,
		InputSchema: schemaFor[WsSendInput](func(s *jsonschema.Schema) {
			if p := prop(s, "frames"); p != nil {
				p.MinItems = intPtr(1)
				p.MaxItems = intPtr(maxWsFrames)
				if p.Items != nil {
					if f := prop(p.Items, "format"); f != nil {
						f.Enum = []any{"TEXT", "BINARY"}
					}
				}
			}
		}),
		Annotations: writeAnn(false, false, true),
	}, wsSendHandler(client))
}

// RegisterWsReadTool registers caido_ws_read.
func RegisterWsReadTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:  "caido_ws_read",
		Title: "WebSocket Replay: Read Frames",
		Description: `Long-poll a session's WebSocket for new frames, for push protocols where the server answers seconds later (e.g. subscriptions fired by another actor). ` +
			`By default returns only server frames, as soon as one arrives or after waitMs (default 3000, max 120000). ` +
			`untilContains / untilCount wait for a specific frame or number of frames; skipBodies drops keep-alives; PING/PONG are always dropped. ` +
			`Pass the returned cursor as after to continue where this read stopped; drop it after reconnecting with caido_ws_connect. more=true means the read hit its scan cap with frames left; read again with cursor. open=false means the connection closed. ` +
			`streamId instead of sessionId reads any recorded run, e.g. the previousStreamId caido_ws_connect returns after reusing a session (includeSent=true shows both directions). ` +
			`skippedSent / skippedBodies / skippedControl count frames left out as client frames, skipBodies matches, and PING/PONG. ` +
			`Caveat (Caido 0.59.1): replay recording drops server frames that arrive back to back (under ~20ms apart), so a burst can show fewer frames than were sent.`,
		Annotations: readOnlyAnn(),
	}, wsReadHandler(client))
}

// RegisterWsCloseTool registers caido_ws_close.
func RegisterWsCloseTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:  "caido_ws_close",
		Title: "WebSocket Replay: Close",
		Description: `Close a session's open WebSocket. Caido closes it without a status code or reason. ` +
			`closed=false means nothing was open; timedOut=true means Caido did not confirm the close within timeoutSec (default 10) and it may still be open. The session and its frames stay in Replay.`,
		Annotations: writeAnn(false, true, true),
	}, wsCloseHandler(client))
}
