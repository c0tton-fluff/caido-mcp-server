package tools

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/c0tton-fluff/caido-mcp-server/internal/httputil"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ExportCurlInput struct {
	RequestID string `json:"requestId" jsonschema:"required,Request ID to export as curl"`
}

type ExportCurlOutput struct {
	Curl  string `json:"curl,omitempty"`
	Error string `json:"error,omitempty"`
}

func escapeSingleQuote(s string) string {
	return strings.ReplaceAll(s, "'", `'\''`)
}

func buildCurlCommand(method, url string, headers []httputil.Header, body string) string {
	var parts []string
	parts = append(parts, "curl")

	if method != "" && method != "GET" {
		parts = append(parts, "-X", method)
	}

	parts = append(parts, fmt.Sprintf("'%s'", escapeSingleQuote(url)))

	for _, h := range headers {
		if strings.ToLower(h.Name) == "host" {
			continue
		}
		headerValue := h.Value
		if headerValue == "[REDACTED]" {
			headerValue = "REDACTED"
		}
		parts = append(parts, "-H", fmt.Sprintf("'%s: %s'",
			escapeSingleQuote(h.Name),
			escapeSingleQuote(headerValue)))
	}

	if body != "" {
		parts = append(parts, "-d", fmt.Sprintf("'%s'", escapeSingleQuote(body)))
	}

	return strings.Join(parts, " ")
}

func exportCurlHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, ExportCurlInput) (*mcp.CallToolResult, ExportCurlOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input ExportCurlInput,
	) (*mcp.CallToolResult, ExportCurlOutput, error) {
		if input.RequestID == "" {
			return nil, ExportCurlOutput{}, fmt.Errorf("requestId is required")
		}

		resp, err := client.Requests.Get(ctx, input.RequestID)
		if err != nil {
			return nil, ExportCurlOutput{
				Error: fmt.Sprintf("failed to get request: %v", err),
			}, nil
		}

		r := resp.Request
		if r == nil {
			return nil, ExportCurlOutput{
				Error: "request not found",
			}, nil
		}

		decoded, err := base64.StdEncoding.DecodeString(r.Raw)
		if err != nil {
			return nil, ExportCurlOutput{
				Error: fmt.Sprintf("failed to decode raw request: %v", err),
			}, nil
		}

		parsed := httputil.ParseRaw(decoded, true, true, 0, 0)
		if parsed == nil {
			return nil, ExportCurlOutput{
				Error: "failed to parse request",
			}, nil
		}

		method := r.Method
		scheme := "http"
		if r.IsTls {
			scheme = "https"
		}

		hostPort := r.Host
		defaultPort := httputil.DefaultPort(r.IsTls)
		if r.Port != 0 && r.Port != defaultPort {
			hostPort = fmt.Sprintf("%s:%d", r.Host, r.Port)
		}

		path := r.Path
		if r.Query != "" {
			path = path + "?" + r.Query
		}

		url := fmt.Sprintf("%s://%s%s", scheme, hostPort, path)

		curl := buildCurlCommand(method, url, parsed.Headers, parsed.Body)

		return nil, ExportCurlOutput{Curl: curl}, nil
	}
}

func RegisterExportCurlTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_export_curl",
		Description: `Convert a Caido request to a curl command. Returns executable curl command string with method, URL, headers, and body.`,
	}, exportCurlHandler(client))
}
