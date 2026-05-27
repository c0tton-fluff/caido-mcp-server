package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/c0tton-fluff/caido-mcp-server/internal/httputil"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type GetInterceptEntryInput struct {
	ID         string `json:"id" jsonschema:"required,Intercept entry ID"`
	BodyLimit  int    `json:"bodyLimit,omitempty" jsonschema:"Response body byte limit (default adaptive)"`
	BodyOffset int    `json:"bodyOffset,omitempty" jsonschema:"Response body byte offset (default 0)"`
}

type GetInterceptEntryOutput struct {
	ID         string                  `json:"id"`
	RequestID  string                  `json:"requestId"`
	Method     string                  `json:"method"`
	URL        string                  `json:"url"`
	CreatedAt  string                  `json:"createdAt"`
	StatusCode int                     `json:"statusCode,omitempty"`
	Request    *httputil.ParsedMessage `json:"request,omitempty"`
	Response   *httputil.ParsedMessage `json:"response,omitempty"`
}

func getInterceptEntryHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, GetInterceptEntryInput) (*mcp.CallToolResult, GetInterceptEntryOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input GetInterceptEntryInput,
	) (*mcp.CallToolResult, GetInterceptEntryOutput, error) {
		if input.ID == "" {
			return nil, GetInterceptEntryOutput{}, fmt.Errorf("id is required")
		}

		resp, err := client.Intercept.GetEntry(ctx, input.ID)
		if err != nil {
			return nil, GetInterceptEntryOutput{}, err
		}

		entry := resp.InterceptEntry
		if entry == nil {
			return nil, GetInterceptEntryOutput{}, fmt.Errorf("intercept entry not found")
		}

		r := entry.Request
		out := GetInterceptEntryOutput{
			ID:        entry.Id,
			RequestID: r.Id,
			Method:    r.Method,
			URL:       httputil.BuildURL(r.IsTls, r.Host, r.Port, r.Path, r.Query),
			CreatedAt: time.UnixMilli(r.CreatedAt).Format(time.RFC3339),
			Request:   httputil.ParseBase64(r.Raw, true, false, 0, 0),
		}

		if r.Response != nil {
			out.StatusCode = r.Response.StatusCode
			bodyLimit := input.BodyLimit
			if bodyLimit == 0 {
				headersOnly := httputil.ParseBase64(r.Response.Raw, true, false, 0, 0)
				if headersOnly != nil && headersOnly.Fingerprint != nil {
					bodyLimit = httputil.AdaptiveBodyLimit(*headersOnly.Fingerprint, 0)
				} else {
					bodyLimit = httputil.DefaultBodyLimit
				}
			}
			out.Response = httputil.ParseBase64(
				r.Response.Raw, true, true, input.BodyOffset, bodyLimit,
			)
		}

		return nil, out, nil
	}
}

func RegisterGetInterceptEntryTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "caido_get_intercept_entry",
		Description: `Get a single intercept entry by ID with its full request ` +
			`(and response, if already received). Use after caido_list_intercept_entries.`,
	}, getInterceptEntryHandler(client))
}
