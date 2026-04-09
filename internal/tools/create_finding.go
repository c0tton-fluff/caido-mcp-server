package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	gen "github.com/caido-community/sdk-go/graphql"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// CreateFindingInput is the input for the create_finding tool
type CreateFindingInput struct {
	RequestID   string  `json:"requestId" jsonschema:"required,ID of the request associated with this finding"`
	Title       string  `json:"title" jsonschema:"required,Title of the finding"`
	Description *string `json:"description,omitempty" jsonschema:"Detailed description of the finding"`
	Reporter    string  `json:"reporter,omitempty" jsonschema:"Reporter name (default: Claude)"`
}

// CreateFindingOutput is the output of the create_finding tool
type CreateFindingOutput struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Host     string `json:"host"`
	Path     string `json:"path"`
	Reporter string `json:"reporter"`
}

// createFindingHandler creates the handler function
func createFindingHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, CreateFindingInput) (*mcp.CallToolResult, CreateFindingOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input CreateFindingInput,
	) (*mcp.CallToolResult, CreateFindingOutput, error) {
		if input.RequestID == "" {
			return nil, CreateFindingOutput{}, fmt.Errorf(
				"requestId is required",
			)
		}
		if input.Title == "" {
			return nil, CreateFindingOutput{}, fmt.Errorf(
				"title is required",
			)
		}
		if len(input.Title) > 500 {
			return nil, CreateFindingOutput{}, fmt.Errorf(
				"title exceeds max length of 500",
			)
		}
		if input.Description != nil && len(*input.Description) > 10000 {
			return nil, CreateFindingOutput{}, fmt.Errorf(
				"description exceeds max length of 10000",
			)
		}

		reporter := input.Reporter
		if reporter == "" {
			reporter = "Claude"
		}

		resp, err := client.Findings.Create(
			ctx,
			input.RequestID,
			&gen.CreateFindingInput{
				Title:       input.Title,
				Description: input.Description,
				Reporter:    reporter,
			},
		)
		if err != nil {
			return nil, CreateFindingOutput{}, err
		}

		payload := resp.CreateFinding
		if payload.Error != nil {
			return nil, CreateFindingOutput{}, fmt.Errorf(
				"create finding failed",
			)
		}

		f := payload.Finding
		return nil, CreateFindingOutput{
			ID:       f.Id,
			Title:    f.Title,
			Host:     f.Host,
			Path:     f.Path,
			Reporter: f.Reporter,
		}, nil
	}
}

// RegisterCreateFindingTool registers the tool with the MCP server
func RegisterCreateFindingTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_create_finding",
		Description: `Create finding. Params: requestId, title, description (optional).`,
	}, createFindingHandler(client))
}
