package tools

import (
	"encoding/base64"
	"fmt"
	"unicode/utf8"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/httputil"
)

// WebSocket frame payloads arrive as base64 Blobs (the GraphQL serialization
// of a byte string). These helpers turn one into the same {body, bodyEncoding,
// bodySize, truncated} envelope the HTTP tools use, so a binary frame is not
// mangled into replacement characters on its way to the model.

// WsFrameView is one WebSocket frame of a replay entry or a stream.
type WsFrameView struct {
	ID           string `json:"id"`
	Direction    string `json:"direction"`
	Format       string `json:"format"`
	Length       int    `json:"length"`
	Body         string `json:"body"`
	BodyEncoding string `json:"bodyEncoding"`
	BodySize     int    `json:"bodySize"`
	Truncated    bool   `json:"truncated,omitempty"`
	CreatedAt    int64  `json:"createdAt,omitempty"`
}

// WsDraftView is an entry's pending message, rendered like a frame.
type WsDraftView struct {
	Direction    string `json:"direction"`
	Format       string `json:"format"`
	Body         string `json:"body"`
	BodyEncoding string `json:"bodyEncoding"`
	BodySize     int    `json:"bodySize"`
	Truncated    bool   `json:"truncated,omitempty"`
}

// renderWsFrame decodes a base64 Blob payload and slices it to limit bytes.
// encoding selects the representation, exactly like bodyEncoding does for
// HTTP bodies: auto keeps UTF-8 text and base64-encodes binary payloads.
func renderWsFrame(
	raw string, limit int, encoding httputil.Encoding,
) (body, bodyEncoding string, size int, truncated bool) {
	payload := []byte(raw)
	if decoded, err := base64.StdEncoding.DecodeString(raw); err == nil {
		payload = decoded
	}
	size = len(payload)
	if limit > 0 && len(payload) > limit {
		payload = payload[:limit]
		truncated = true
	}
	switch encoding {
	case httputil.EncodingBase64:
		return base64.StdEncoding.EncodeToString(payload), "base64", size, truncated
	case httputil.EncodingText:
		return string(payload), "text", size, truncated
	}
	if utf8.Valid(payload) {
		return string(payload), "text", size, truncated
	}
	return base64.StdEncoding.EncodeToString(payload), "base64", size, truncated
}

// encodeWsFrame turns an agent-supplied payload into the base64 Blob Caido
// stores. encoding is "text" (default) or "base64".
func encodeWsFrame(payload, encoding string) (string, error) {
	if encoding == "base64" {
		if _, err := base64.StdEncoding.DecodeString(payload); err != nil {
			return "", fmt.Errorf("payload is not valid base64: %w", err)
		}
		return payload, nil
	}
	return base64.StdEncoding.EncodeToString([]byte(payload)), nil
}
