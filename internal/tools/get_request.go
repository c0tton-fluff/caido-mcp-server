package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/c0tton-fluff/caido-mcp-server/internal/caido"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GetRequestInput is the input for the get_request tool
type GetRequestInput struct {
	// Request IDs to retrieve
	IDs []string `json:"ids" jsonschema:"required,Request IDs"`
	// Fields to include. Default: metadata only. Options: metadata, requestHeaders, requestBody, responseHeaders, responseBody
	Include []string `json:"include,omitempty" jsonschema:"Fields to include (default: metadata only)"`
	// Offset for body content (bytes to skip)
	BodyOffset int `json:"bodyOffset,omitempty" jsonschema:"Body byte offset"`
	// Limit for body content (default 2000 bytes)
	BodyLimit int `json:"bodyLimit,omitempty" jsonschema:"Body byte limit (default 2000)"`
}

// ParsedHTTPMessage contains parsed headers and body
type ParsedHTTPMessage struct {
	FirstLine string            `json:"firstLine,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Body      string            `json:"body,omitempty"`
	BodySize  int               `json:"bodySize,omitempty"`
	Truncated bool              `json:"truncated,omitempty"`
}

// GetRequestOutput is the output for a single request
type GetRequestOutput struct {
	ID          string             `json:"id"`
	Method      string             `json:"method,omitempty"`
	Host        string             `json:"host,omitempty"`
	Port        int                `json:"port,omitempty"`
	Path        string             `json:"path,omitempty"`
	Query       string             `json:"query,omitempty"`
	IsTLS       bool               `json:"isTls,omitempty"`
	StatusCode  int                `json:"statusCode,omitempty"`
	RoundtripMs int                `json:"roundtripMs,omitempty"`
	CreatedAt   string             `json:"createdAt,omitempty"`
	Request     *ParsedHTTPMessage `json:"request,omitempty"`
	Response    *ParsedHTTPMessage `json:"response,omitempty"`
	Error       string             `json:"error,omitempty"`
}

// GetRequestBatchOutput is the output for batch requests
type GetRequestBatchOutput struct {
	Requests []GetRequestOutput `json:"requests"`
}

// parseHTTPMessage parses raw HTTP message (base64 encoded) into headers and body
func parseHTTPMessage(rawBase64 string, includeHeaders, includeBody bool, bodyOffset, bodyLimit int) *ParsedHTTPMessage {
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
		// Apply offset
		if bodyOffset > 0 {
			if bodyOffset >= len(bodyPart) {
				bodyPart = []byte{}
			} else {
				bodyPart = bodyPart[bodyOffset:]
			}
		}

		// Apply limit
		if bodyLimit > 0 && len(bodyPart) > bodyLimit {
			bodyPart = bodyPart[:bodyLimit]
			result.Truncated = true
		}

		result.Body = string(bodyPart)
	}

	return result
}

// shouldInclude checks if a field should be included
func shouldInclude(include []string, field string) bool {
	if len(include) == 0 {
		// Only return metadata by default to save context tokens
		// Use include=["requestHeaders","requestBody","responseHeaders","responseBody"] for full data
		return field == "metadata"
	}
	for _, f := range include {
		if f == field {
			return true
		}
	}
	return false
}

// processRequest converts a caido.Request to GetRequestOutput with field selection
func processRequest(request *caido.Request, include []string, bodyOffset, bodyLimit int) GetRequestOutput {
	output := GetRequestOutput{
		ID: request.ID,
	}

	// Metadata
	if shouldInclude(include, "metadata") || len(include) == 0 {
		output.Method = request.Method
		output.Host = request.Host
		output.Port = request.Port
		output.Path = request.Path
		output.Query = request.Query
		output.IsTLS = request.IsTLS
		output.CreatedAt = request.CreatedAt.Time().Format(time.RFC3339)
		if request.Response != nil {
			output.StatusCode = request.Response.StatusCode
			output.RoundtripMs = request.Response.RoundtripTime
		}
	}

	// Request headers/body
	includeReqHeaders := shouldInclude(include, "requestHeaders")
	includeReqBody := shouldInclude(include, "requestBody")
	if includeReqHeaders || includeReqBody {
		output.Request = parseHTTPMessage(request.Raw, includeReqHeaders, includeReqBody, bodyOffset, bodyLimit)
	}

	// Response headers/body
	if request.Response != nil {
		includeRespHeaders := shouldInclude(include, "responseHeaders")
		includeRespBody := shouldInclude(include, "responseBody")
		if includeRespHeaders || includeRespBody {
			output.Response = parseHTTPMessage(request.Response.Raw, includeRespHeaders, includeRespBody, bodyOffset, bodyLimit)
		}
	}

	return output
}

// getRequestHandler creates the handler function for the get_request tool
func getRequestHandler(client *caido.Client) func(context.Context, *mcp.CallToolRequest, GetRequestInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input GetRequestInput) (*mcp.CallToolResult, any, error) {
		if len(input.IDs) == 0 {
			return nil, nil, fmt.Errorf("at least one request ID is required")
		}

		include := input.Include

		// Default body limit to save context tokens (2KB)
		bodyLimit := input.BodyLimit
		if bodyLimit == 0 {
			bodyLimit = 2000
		}

		// Fetch requests
		var results []GetRequestOutput
		for _, id := range input.IDs {
			request, err := client.GetRequest(ctx, id)
			if err != nil {
				results = append(results, GetRequestOutput{
					ID:    id,
					Error: fmt.Sprintf("failed to get request: %v", err),
				})
				continue
			}

			output := processRequest(request, include, input.BodyOffset, bodyLimit)
			results = append(results, output)
		}

		// Return single object for single request
		if len(input.IDs) == 1 {
			return nil, results[0], nil
		}

		// Return batch response
		return nil, GetRequestBatchOutput{Requests: results}, nil
	}
}

// RegisterGetRequestTool registers the tool with the MCP server
func RegisterGetRequestTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_get_request",
		Description: `Get request details. Default: metadata only (saves tokens). Use include=[requestHeaders,requestBody,responseHeaders,responseBody] for more. Body limit: 2KB default.`,
	}, getRequestHandler(client))
}
