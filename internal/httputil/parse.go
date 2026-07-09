package httputil

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"io"
	"os"
	"strconv"
	"strings"
)

const DefaultBodyLimit = 2000

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

type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type ParsedMessage struct {
	FirstLine   string       `json:"firstLine,omitempty"`
	Headers     []Header     `json:"headers,omitempty"`
	Body        string       `json:"body,omitempty"`
	BodySize    int          `json:"bodySize,omitempty"`
	Truncated   bool         `json:"truncated,omitempty"`
	Fingerprint *Fingerprint `json:"fingerprint,omitempty"`
}

func ParseBase64(
	raw string,
	includeHeaders, includeBody bool,
	bodyOffset, bodyLimit int,
) *ParsedMessage {
	if raw == "" {
		return nil
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil
	}
	return ParseRaw(
		decoded, includeHeaders, includeBody, bodyOffset, bodyLimit,
	)
}

func ParseRaw(
	raw []byte,
	includeHeaders, includeBody bool,
	bodyOffset, bodyLimit int,
) *ParsedMessage {
	result := &ParsedMessage{}
	parts := bytes.SplitN(raw, []byte("\r\n\r\n"), 2)
	if len(parts) == 1 {
		// Fallback for messages using \n\n instead of \r\n\r\n
		parts = bytes.SplitN(raw, []byte("\n\n"), 2)
	}

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
	} else {
		fp := Fingerprint{BodySize: 0, Kind: KindEmpty}
		result.Fingerprint = &fp
	}

	if includeBody && len(bodyPart) > 0 {
		if bodyOffset > 0 {
			if bodyOffset >= len(bodyPart) {
				bodyPart = []byte{}
			} else {
				bodyPart = bodyPart[bodyOffset:]
			}
		}
		if bodyLimit > 0 && len(bodyPart) > bodyLimit {
			bodyPart = bodyPart[:bodyLimit]
			result.Truncated = true
		}
		result.Body = string(bodyPart)
	}

	return result
}
