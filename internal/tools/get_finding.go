package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/c0tton-fluff/caido-mcp-server/internal/httputil"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type GetFindingInput struct {
	ID string `json:"id" jsonschema:"required,Finding ID"`
}

type GetFindingOutput struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Description *string `json:"description,omitempty"`
	Reporter    string  `json:"reporter"`
	Host        string  `json:"host"`
	Path        string  `json:"path"`
	Hidden      bool    `json:"hidden"`
	DedupeKey   *string `json:"dedupeKey,omitempty"`
	CreatedAt   string  `json:"createdAt"`
	RequestID   string  `json:"requestId"`
	RequestURL  string  `json:"requestUrl"`
}

func getFindingHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, GetFindingInput) (*mcp.CallToolResult, GetFindingOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input GetFindingInput,
	) (*mcp.CallToolResult, GetFindingOutput, error) {
		resp, err := client.Findings.Get(ctx, input.ID)
		if err != nil {
			return nil, GetFindingOutput{}, err
		}

		f := resp.Finding
		if f == nil {
			return nil, GetFindingOutput{}, fmt.Errorf("finding not found")
		}

		return nil, GetFindingOutput{
			ID:          f.Id,
			Title:       f.Title,
			Description: f.Description,
			Reporter:    f.Reporter,
			Host:        f.Host,
			Path:        f.Path,
			Hidden:      f.Hidden,
			DedupeKey:   f.DedupeKey,
			CreatedAt:   time.UnixMilli(f.CreatedAt).Format(time.RFC3339),
			RequestID:   f.Request.Id,
			RequestURL: httputil.BuildURL(
				f.Request.IsTls, f.Request.Host, f.Request.Port,
				f.Request.Path, f.Request.Query,
			),
		}, nil
	}
}

func RegisterGetFindingTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "caido_get_finding",
		Description: `Get a single security finding by ID: title, description, ` +
			`reporter, and the request it is attached to.`,
	}, getFindingHandler(client))
}
