package tools

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"io"
	"strings"
)

// ParsedHTTPMessage contains parsed headers and body
type ParsedHTTPMessage struct {
	FirstLine string            `json:"firstLine,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Body      string            `json:"body,omitempty"`
	BodySize  int               `json:"bodySize,omitempty"`
	Truncated bool              `json:"truncated,omitempty"`
}

// parseHTTPMessage parses raw HTTP message (base64 encoded)
// into headers and body
func parseHTTPMessage(
	rawBase64 string,
	includeHeaders, includeBody bool,
	bodyOffset, bodyLimit int,
) *ParsedHTTPMessage {
	if rawBase64 == "" {
		return nil
	}

	raw, err := base64.StdEncoding.DecodeString(rawBase64)
	if err != nil {
		return nil
	}

	result := &ParsedHTTPMessage{}

	// Split headers and body
	parts := bytes.SplitN(raw, []byte("\r\n\r\n"), 2)
	headerPart := parts[0]
	var bodyPart []byte
	if len(parts) > 1 {
		bodyPart = parts[1]
	}

	// Parse headers
	if includeHeaders {
		result.Headers = make(map[string]string)
		reader := bufio.NewReader(bytes.NewReader(headerPart))

		// First line (request line or status line)
		firstLine, err := reader.ReadString('\n')
		if err == nil || err == io.EOF {
			result.FirstLine = strings.TrimSpace(firstLine)
		}

		// Read headers
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				break
			}
			line = strings.TrimSpace(line)
			if line == "" {
				break
			}
			colonIdx := strings.Index(line, ":")
			if colonIdx > 0 {
				key := strings.TrimSpace(line[:colonIdx])
				value := strings.TrimSpace(line[colonIdx+1:])
				result.Headers[key] = value
			}
		}
	}

	// Parse body
	result.BodySize = len(bodyPart)
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
