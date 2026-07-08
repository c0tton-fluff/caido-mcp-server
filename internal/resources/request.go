package resources

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/c0tton-fluff/caido-mcp-server/internal/httputil"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerRequestResource(server *mcp.Server, client *caido.Client) {
	server.AddResourceTemplate(
		&mcp.ResourceTemplate{
			URITemplate: "caido://requests/{id}",
			Name:        "caido-request",
			Description: "HTTP request and response captured by Caido proxy",
			MIMEType:    "text/plain",
		},
		requestHandler(client),
	)
}

func requestHandler(client *caido.Client) mcp.ResourceHandler {
	return func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		id := extractIDFromURI(req.Params.URI, "caido://requests/")
		if id == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}

		resp, err := client.Requests.Get(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("get request %s: %w", id, err)
		}
		if resp.Request == nil {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}

		r := resp.Request
		var b strings.Builder
		fmt.Fprintf(&b, "# Request %s\n", r.Id)
		fmt.Fprintf(&b, "Method: %s\n", r.Method)
		fmt.Fprintf(&b, "URL: %s://%s:%d%s", scheme(r.IsTls), r.Host, r.Port, r.Path)
		if r.Query != "" {
			fmt.Fprintf(&b, "?%s", r.Query)
		}
		b.WriteString("\n\n")

		if r.Raw != "" {
			decoded, err := base64.StdEncoding.DecodeString(r.Raw)
			if err == nil {
				fmt.Fprintf(&b, "## Raw Request\n```http\n%s\n```\n\n", truncate(httputil.RedactRawHeaders(string(decoded)), 4096))
			}
		}

		if r.Response != nil {
			fmt.Fprintf(&b, "## Response\n")
			fmt.Fprintf(&b, "Status: %d\n", r.Response.StatusCode)
			fmt.Fprintf(&b, "Roundtrip: %dms\n", r.Response.RoundtripTime)
			if r.Response.Raw != "" {
				decoded, err := base64.StdEncoding.DecodeString(r.Response.Raw)
				if err == nil {
					fmt.Fprintf(&b, "\n```http\n%s\n```\n", truncate(httputil.RedactRawHeaders(string(decoded)), 4096))
				}
			}
		}

		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{
				URI:  req.Params.URI,
				Text: b.String(),
			}},
		}, nil
	}
}

func scheme(isTLS bool) string {
	if isTLS {
		return "https"
	}
	return "http"
}

func truncate(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	return s[:maxBytes] + "\n... (truncated)"
}

func extractIDFromURI(uri, prefix string) string {
	if !strings.HasPrefix(uri, prefix) {
		return ""
	}
	return strings.TrimPrefix(uri, prefix)
}
