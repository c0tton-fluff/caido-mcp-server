package tools

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/httputil"
	caido "github.com/caido-community/sdk-go"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GetRequestInput is the input for the get_request tool
type GetRequestInput struct {
	IDs        []string `json:"ids" jsonschema:"required,Request IDs"`
	Include    []string `json:"include,omitempty" jsonschema:"Fields to include (default: metadata only)"`
	BodyOffset int      `json:"bodyOffset,omitempty" jsonschema:"Body byte offset"`
	BodyLimit  int      `json:"bodyLimit,omitempty" jsonschema:"Body byte limit (default 2000)"`
	// BodyEncoding forces how bodies are rendered. Empty means auto: UTF-8
	// bodies stay text (the previous output), binary bodies become base64.
	BodyEncoding string `json:"bodyEncoding,omitempty" jsonschema:"Body text or base64: auto (default; text for UTF-8 bodies, base64 for binary), text, base64"`
}

// GetRequestOutput is the output for a single request
type GetRequestOutput struct {
	ID          string                  `json:"id"`
	Method      string                  `json:"method,omitempty"`
	Host        string                  `json:"host,omitempty"`
	Port        int                     `json:"port,omitempty"`
	Path        string                  `json:"path,omitempty"`
	Query       string                  `json:"query,omitempty"`
	IsTLS       bool                    `json:"isTls,omitempty"`
	StatusCode  int                     `json:"statusCode,omitempty"`
	RoundtripMs int                     `json:"roundtripMs,omitempty"`
	CreatedAt   string                  `json:"createdAt,omitempty"`
	Request     *httputil.ParsedMessage `json:"request,omitempty"`
	Response    *httputil.ParsedMessage `json:"response,omitempty"`
	Error       string                  `json:"error,omitempty"`
}

// GetRequestBatchOutput is the output for batch requests
type GetRequestBatchOutput struct {
	Requests []GetRequestOutput `json:"requests"`
}

func shouldInclude(include []string, field string) bool {
	if len(include) == 0 {
		return field == "metadata"
	}
	return slices.Contains(include, field)
}

func includeRequiresRaw(include []string) bool {
	return shouldInclude(include, "requestHeaders") ||
		shouldInclude(include, "requestBody") ||
		shouldInclude(include, "responseHeaders") ||
		shouldInclude(include, "responseBody")
}

// getRequestHandler creates the handler function for the get_request tool
func getRequestHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, GetRequestInput) (*mcp.CallToolResult, any, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input GetRequestInput,
	) (*mcp.CallToolResult, any, error) {
		if len(input.IDs) == 0 {
			return nil, nil, fmt.Errorf(
				"at least one request ID is required",
			)
		}
		if len(input.IDs) > 20 {
			return nil, nil, fmt.Errorf(
				"max 20 request IDs per call",
			)
		}

		include := input.Include
		bodyLimit := input.BodyLimit
		if bodyLimit == 0 {
			bodyLimit = httputil.DefaultBodyLimit
		}
		encoding, err := httputil.ParseEncoding(input.BodyEncoding)
		if err != nil {
			return nil, nil, err
		}

		var results []GetRequestOutput
		for _, id := range input.IDs {
			var output GetRequestOutput
			if includeRequiresRaw(include) {
				output = processFullRequest(
					ctx, client, id, include,
					input.BodyOffset, bodyLimit, encoding,
				)
			} else {
				output = processMetadataRequest(
					ctx, client, id, include,
				)
			}
			results = append(results, output)
		}

		if len(input.IDs) == 1 {
			return nil, results[0], nil
		}
		return nil, GetRequestBatchOutput{Requests: results}, nil
	}
}

func processMetadataRequest(
	ctx context.Context,
	client *caido.Client,
	id string,
	include []string,
) GetRequestOutput {
	resp, err := client.Requests.GetMetadata(ctx, id)
	if err != nil {
		return GetRequestOutput{
			ID:    id,
			Error: fmt.Sprintf("failed to get request: %v", err),
		}
	}

	r := resp.Request
	if r == nil {
		return GetRequestOutput{
			ID:    id,
			Error: "request not found",
		}
	}

	output := GetRequestOutput{ID: r.Id}
	if shouldInclude(include, "metadata") || len(include) == 0 {
		output.Method = r.Method
		output.Host = r.Host
		output.Port = r.Port
		output.Path = r.Path
		output.Query = r.Query
		output.IsTLS = r.IsTls
		output.CreatedAt = time.UnixMilli(r.CreatedAt).Format(
			time.RFC3339,
		)
		if r.Response != nil {
			output.StatusCode = r.Response.StatusCode
			output.RoundtripMs = r.Response.RoundtripTime
		}
	}
	return output
}

func processFullRequest(
	ctx context.Context,
	client *caido.Client,
	id string,
	include []string,
	bodyOffset, bodyLimit int,
	encoding httputil.Encoding,
) GetRequestOutput {
	resp, err := client.Requests.Get(ctx, id)
	if err != nil {
		return GetRequestOutput{
			ID:    id,
			Error: fmt.Sprintf("failed to get request: %v", err),
		}
	}

	r := resp.Request
	if r == nil {
		return GetRequestOutput{
			ID:    id,
			Error: "request not found",
		}
	}

	output := GetRequestOutput{ID: r.Id}
	if shouldInclude(include, "metadata") || len(include) == 0 {
		output.Method = r.Method
		output.Host = r.Host
		output.Port = r.Port
		output.Path = r.Path
		output.Query = r.Query
		output.IsTLS = r.IsTls
		output.CreatedAt = time.UnixMilli(r.CreatedAt).Format(
			time.RFC3339,
		)
		if r.Response != nil {
			output.StatusCode = r.Response.StatusCode
			output.RoundtripMs = r.Response.RoundtripTime
		}
	}

	inclReqH := shouldInclude(include, "requestHeaders")
	inclReqB := shouldInclude(include, "requestBody")
	if inclReqH || inclReqB {
		output.Request = httputil.ParseBase64With(
			r.Raw, inclReqH, inclReqB, bodyOffset, bodyLimit, encoding,
		)
	}

	if r.Response != nil {
		inclRespH := shouldInclude(include, "responseHeaders")
		inclRespB := shouldInclude(include, "responseBody")
		if inclRespH || inclRespB {
			output.Response = httputil.ParseBase64With(
				r.Response.Raw, inclRespH, inclRespB,
				bodyOffset, bodyLimit, encoding,
			)
		}
	}

	return output
}

// RegisterGetRequestTool registers the tool with the MCP server
func RegisterGetRequestTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_get_request",
		Title:       "Get Request",
		Description: `Get request details. Default: metadata only (saves tokens). Use include=[requestHeaders,requestBody,responseHeaders,responseBody] for more. Body limit: 2KB default; bodyOffset/bodyLimit count bytes. Bodies come back as text when they are UTF-8 and base64 with bodyEncoding:"base64" (plus bodySize, bodyOffset) when they are not, so binary responses survive the JSON round trip; set bodyEncoding to force text or base64.`,
		InputSchema: schemaFor[GetRequestInput](func(s *jsonschema.Schema) {
			if p := prop(s, "bodyEncoding"); p != nil {
				p.Enum = []any{"auto", "text", "base64"}
			}
		}),
		Annotations: readOnlyAnn(),
	}, getRequestHandler(client))
}
