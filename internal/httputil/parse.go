package httputil

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"
)

const DefaultBodyLimit = 2000

// Encoding selects how a parsed body is rendered.
type Encoding string

const (
	// EncodingAuto returns the body as text when the selected bytes are UTF-8
	// text and base64-encodes anything else. A body that is not valid UTF-8
	// cannot survive a JSON string, where invalid sequences become U+FFFD, so
	// binary content (images, protobuf, compressed bodies) is handed over as
	// bytes instead of as a lossy copy.
	EncodingAuto Encoding = "auto"
	// EncodingText always returns a string, replacing bytes that are not
	// valid UTF-8 the way JSON does. This is what every caller did before
	// Encoding existed.
	EncodingText Encoding = "text"
	// EncodingBase64 always returns base64, even for text bodies.
	EncodingBase64 Encoding = "base64"
)

// ParseEncoding validates an agent-facing encoding value. Empty means auto.
func ParseEncoding(v string) (Encoding, error) {
	switch Encoding(strings.ToLower(strings.TrimSpace(v))) {
	case "", EncodingAuto:
		return EncodingAuto, nil
	case EncodingText:
		return EncodingText, nil
	case EncodingBase64:
		return EncodingBase64, nil
	default:
		return "", fmt.Errorf(
			"bodyEncoding must be one of [auto text base64], got %q", v,
		)
	}
}

// sensitiveHeaders are redacted in tool output to prevent
// leaking credentials to the LLM context.
var sensitiveHeaders = map[string]bool{
	"authorization":       true,
	"cookie":              true,
	"set-cookie":          true,
	"proxy-authorization": true,
	"x-api-key":           true,
	"x-auth-token":        true,
	"x-csrf-token":        true,
	"x-xsrf-token":        true,
}

// allowSensitiveHeaders reports whether sensitive-header redaction is disabled.
// Opt in by setting CAIDO_ALLOW_SENSITIVE_HEADERS to a truthy value accepted by
// strconv.ParseBool (1, t, T, TRUE, true, True); unset, empty, or unparseable
// values keep the default redact-everything behavior. Read per ParseRaw call so
// it stays testable and runtime-configurable. Intended for authorized
// proxy/pentest work where replaying a captured authenticated request needs its
// real Authorization/Cookie/CSRF headers.
func allowSensitiveHeaders() bool {
	allow, _ := strconv.ParseBool(os.Getenv("CAIDO_ALLOW_SENSITIVE_HEADERS"))
	return allow
}

// Redacted is what a withheld credential value is replaced with.
const Redacted = "[REDACTED]"

// SensitiveValuesAllowed reports whether the operator opted out of
// credential redaction (CAIDO_ALLOW_SENSITIVE_HEADERS). It is the one switch
// for every place a stored credential could reach the model: sensitive
// headers here, SECRET environment variables in the tools package.
func SensitiveValuesAllowed() bool { return allowSensitiveHeaders() }

type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type ParsedMessage struct {
	FirstLine string   `json:"firstLine,omitempty"`
	Headers   []Header `json:"headers,omitempty"`
	Body      string   `json:"body,omitempty"`
	// BodyBase64 carries the selected body bytes when a body is returned
	// base64-encoded; Body is empty in that case.
	BodyBase64 string `json:"bodyBase64,omitempty"`
	// BodyEncoding is set to "base64" only for base64 bodies. Text bodies
	// carry no encoding field, so their output is unchanged.
	BodyEncoding string `json:"bodyEncoding,omitempty"`
	// BodyOffset is the byte offset of the returned slice within the body.
	// It is part of the base64 envelope only.
	BodyOffset  *int         `json:"bodyOffset,omitempty"`
	BodySize    int          `json:"bodySize,omitempty"`
	Truncated   bool         `json:"truncated,omitempty"`
	Fingerprint *Fingerprint `json:"fingerprint,omitempty"`
}

func ParseBase64(
	raw string,
	includeHeaders, includeBody bool,
	bodyOffset, bodyLimit int,
) *ParsedMessage {
	return ParseBase64With(
		raw, includeHeaders, includeBody, bodyOffset, bodyLimit,
		EncodingText,
	)
}

// ParseBase64With is ParseBase64 with an explicit body encoding.
func ParseBase64With(
	raw string,
	includeHeaders, includeBody bool,
	bodyOffset, bodyLimit int,
	encoding Encoding,
) *ParsedMessage {
	if raw == "" {
		return nil
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil
	}
	return ParseRawWith(
		decoded, includeHeaders, includeBody, bodyOffset, bodyLimit,
		encoding,
	)
}

func ParseRaw(
	raw []byte,
	includeHeaders, includeBody bool,
	bodyOffset, bodyLimit int,
) *ParsedMessage {
	return ParseRawWith(
		raw, includeHeaders, includeBody, bodyOffset, bodyLimit,
		EncodingText,
	)
}

// ParseRawWith parses a raw HTTP message, rendering the body slice according
// to encoding. bodyOffset and bodyLimit count bytes.
func ParseRawWith(
	raw []byte,
	includeHeaders, includeBody bool,
	bodyOffset, bodyLimit int,
	encoding Encoding,
) *ParsedMessage {
	result := &ParsedMessage{}
	parts := bytes.SplitN(raw, []byte("\r\n\r\n"), 2)
	headerPart := parts[0]
	var bodyPart []byte
	if len(parts) > 1 {
		bodyPart = parts[1]
	}

	if includeHeaders {
		allowSensitive := allowSensitiveHeaders()
		reader := bufio.NewReader(bytes.NewReader(headerPart))
		firstLine, err := reader.ReadString('\n')
		if err == nil || err == io.EOF {
			result.FirstLine = strings.TrimSpace(firstLine)
		}
		for {
			line, err := reader.ReadString('\n')
			line = strings.TrimSpace(line)
			if line != "" {
				if idx := strings.Index(line, ":"); idx > 0 {
					name := strings.TrimSpace(line[:idx])
					value := strings.TrimSpace(line[idx+1:])
					if !allowSensitive && sensitiveHeaders[strings.ToLower(name)] {
						value = "[REDACTED]"
					}
					result.Headers = append(result.Headers, Header{
						Name:  name,
						Value: value,
					})
				}
			}
			if err != nil {
				break
			}
		}
	}

	result.BodySize = len(bodyPart)

	if len(result.Headers) > 0 {
		fp := FingerprintFromHeaders(result.Headers, len(bodyPart))
		result.Fingerprint = &fp
	} else if len(bodyPart) > 0 {
		fp := FingerprintFromBody(bodyPart)
		result.Fingerprint = &fp
	}

	if includeBody && len(bodyPart) > 0 {
		start := min(max(bodyOffset, 0), len(bodyPart))
		end := len(bodyPart)
		if bodyLimit > 0 && end-start > bodyLimit {
			end = start + bodyLimit
		}

		mode := encoding
		if mode == "" {
			mode = EncodingAuto
		}
		// The text candidate is the same slice moved to rune boundaries, so
		// a text body cut mid-rune is still text.
		textStart, textEnd := runeBoundary(bodyPart, start), runeBoundary(bodyPart, end)
		text := bodyPart[textStart:textEnd]
		if mode == EncodingAuto {
			if useBase64(text, contentType(headerPart)) {
				mode = EncodingBase64
			} else {
				mode = EncodingText
			}
		}

		if mode == EncodingBase64 {
			result.BodyBase64 = base64.StdEncoding.EncodeToString(
				bodyPart[start:end],
			)
			result.BodyEncoding = string(EncodingBase64)
			offset := start
			result.BodyOffset = &offset
			result.Truncated = end < len(bodyPart)
		} else {
			result.Body = string(text)
			result.Truncated = textEnd < len(bodyPart)
		}
	}

	return result
}

// useBase64 reports whether auto mode should return the body as base64.
// Bytes that are not valid UTF-8 are the primary signal: base64 is the only
// lossless option there. A binary media type is a second signal, but only when
// the bytes also carry NULs: plenty of payloads served as octet-stream or
// protobuf are plain ASCII, and returning those as text is both what agents
// want and what earlier versions did.
func useBase64(body []byte, mediaType string) bool {
	if !utf8.Valid(body) {
		return true
	}
	return bytes.IndexByte(body, 0) >= 0 && binaryMediaType(mediaType)
}

// contentType returns the message's Content-Type value without the parameter
// list, or "" when the message carries none.
func contentType(headers []byte) string {
	reader := bufio.NewReader(bytes.NewReader(headers))
	for {
		line, err := reader.ReadString('\n')
		if idx := strings.IndexByte(line, ':'); idx > 0 {
			if strings.EqualFold(strings.TrimSpace(line[:idx]), "Content-Type") {
				return strings.TrimSpace(line[idx+1:])
			}
		}
		if err != nil {
			return ""
		}
	}
}

// binaryMediaType reports whether a media type always describes bytes rather
// than text. Text-like types are excluded on purpose: image/svg+xml, JSON and
// XML are text even when their type is image/* or has a +json/+xml suffix.
func binaryMediaType(mediaType string) bool {
	if i := strings.IndexByte(mediaType, ';'); i >= 0 {
		mediaType = mediaType[:i]
	}
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	if mediaType == "" {
		return false
	}
	if strings.HasPrefix(mediaType, "text/") ||
		strings.HasSuffix(mediaType, "+json") ||
		strings.HasSuffix(mediaType, "+xml") {
		return false
	}
	switch mediaType {
	case "image/svg+xml", "application/json", "application/xml",
		"application/javascript", "application/x-javascript",
		"application/x-www-form-urlencoded", "application/graphql":
		return false
	}
	if strings.HasPrefix(mediaType, "image/") ||
		strings.HasPrefix(mediaType, "audio/") ||
		strings.HasPrefix(mediaType, "video/") ||
		strings.HasPrefix(mediaType, "font/") {
		return true
	}
	switch mediaType {
	case "application/octet-stream", "application/pdf", "application/zip",
		"application/gzip", "application/wasm", "application/protobuf",
		"application/x-protobuf", "application/x-7z-compressed",
		"application/x-rar-compressed", "application/x-bzip2":
		return true
	}
	return false
}

// runeBoundary moves a byte position that falls inside a UTF-8 rune to the
// end of that rune, and returns every other position unchanged. Applied to
// both ends of a text slice it gives each rune to the page it starts in: a
// rune cut by bodyLimit is returned whole (up to three bytes past the limit)
// and the next page, starting inside it, skips the rest. Paging a text body by
// bodyOffset += bodyLimit therefore returns every rune exactly once.
//
// Only a valid rune that really straddles pos moves it. Stray continuation
// bytes and other invalid sequences are left where they are, so binary data
// is never shortened.
func runeBoundary(body []byte, pos int) int {
	if pos <= 0 || pos >= len(body) || utf8.RuneStart(body[pos]) {
		return pos
	}
	for i := pos - 1; i >= 0 && i > pos-utf8.UTFMax; i-- {
		if !utf8.RuneStart(body[i]) {
			continue
		}
		// An invalid sequence decodes with size 1, which never reaches pos.
		if _, size := utf8.DecodeRune(body[i:]); i+size > pos {
			return i + size
		}
		return pos
	}
	return pos
}
