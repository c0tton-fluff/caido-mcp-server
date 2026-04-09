package httputil

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"io"
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

type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type ParsedMessage struct {
	FirstLine string   `json:"firstLine,omitempty"`
	Headers   []Header `json:"headers,omitempty"`
	Body      string   `json:"body,omitempty"`
	BodySize  int      `json:"bodySize,omitempty"`
	Truncated bool     `json:"truncated,omitempty"`
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
	headerPart := parts[0]
	var bodyPart []byte
	if len(parts) > 1 {
		bodyPart = parts[1]
	}

	if includeHeaders {
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
					if sensitiveHeaders[strings.ToLower(name)] {
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
