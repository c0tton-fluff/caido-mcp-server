package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/c0tton-fluff/caido-mcp-server/internal/httputil"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type GetRequestMetadataInput struct {
	ID string `json:"id" jsonschema:"required,Request ID"`
}

type GetRequestMetadataOutput struct {
	ID            string `json:"id"`
	Method        string `json:"method"`
	URL           string `json:"url"`
	Source        string `json:"source"`
	Alteration    string `json:"alteration"`
	Length        int    `json:"length"`
	CreatedAt     string `json:"createdAt"`
	StatusCode    int    `json:"statusCode,omitempty"`
	RoundtripMs   int    `json:"roundtripMs,omitempty"`
	ResponseBytes int    `json:"responseBytes,omitempty"`
}

func getRequestMetadataHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, GetRequestMetadataInput) (*mcp.CallToolResult, GetRequestMetadataOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input GetRequestMetadataInput,
	) (*mcp.CallToolResult, GetRequestMetadataOutput, error) {
		resp, err := client.Requests.GetMetadata(ctx, input.ID)
		if err != nil {
			return nil, GetRequestMetadataOutput{}, err
		}

		r := resp.Request
		if r == nil {
			return nil, GetRequestMetadataOutput{}, fmt.Errorf("request not found")
		}

		out := GetRequestMetadataOutput{
			ID:         r.Id,
			Method:     r.Method,
			URL:        httputil.BuildURL(r.IsTls, r.Host, r.Port, r.Path, r.Query),
			Source:     string(r.Source),
			Alteration: string(r.Alteration),
			Length:     r.Length,
			CreatedAt:  time.UnixMilli(r.CreatedAt).Format(time.RFC3339),
		}
		if r.Response != nil {
			out.StatusCode = r.Response.StatusCode
			out.RoundtripMs = r.Response.RoundtripTime
			out.ResponseBytes = r.Response.Length
		}
		return nil, out, nil
	}
}

func RegisterGetRequestMetadataTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "caido_get_request_metadata",
		Description: `Get request metadata (method, url, status, timing, sizes) WITHOUT ` +
			`raw bodies. Lighter than caido_get_request when you only need summary info.`,
	}, getRequestMetadataHandler(client))
}
