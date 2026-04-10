package tools

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/c0tton-fluff/caido-mcp-server/internal/httputil"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GetRequestInput is the input for the get_request tool
type GetRequestInput struct {
	IDs        []string `json:"ids" jsonschema:"required,Request IDs"`
	Include    []string `json:"include,omitempty" jsonschema:"Fields to include (default: metadata only)"`
	BodyOffset int      `json:"bodyOffset,omitempty" jsonschema:"Body byte offset"`
	BodyLimit  int      `json:"bodyLimit,omitempty" jsonschema:"Body byte limit (default 2000)"`
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

		var results []GetRequestOutput
		for _, id := range input.IDs {
			var output GetRequestOutput
			if includeRequiresRaw(include) {
				output = processFullRequest(
					ctx, client, id, include,
					input.BodyOffset, bodyLimit,
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
		output.Request = httputil.ParseBase64(
			r.Raw, inclReqH, inclReqB, bodyOffset, bodyLimit,
		)
	}

	if r.Response != nil {
		inclRespH := shouldInclude(include, "responseHeaders")
		inclRespB := shouldInclude(include, "responseBody")
		if inclRespH || inclRespB {
			output.Response = httputil.ParseBase64(
				r.Response.Raw, inclRespH, inclRespB,
				bodyOffset, bodyLimit,
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
		Description: `Get request details. Default: metadata only (saves tokens). Use include=[requestHeaders,requestBody,responseHeaders,responseBody] for more. Body limit: 2KB default.`,
	}, getRequestHandler(client))
}
