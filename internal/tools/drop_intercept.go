package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// DropInterceptInput is the input for the tool
type DropInterceptInput struct {
	ID string `json:"id" jsonschema:"required,Intercept entry ID to drop"`
}

// DropInterceptOutput is the output
type DropInterceptOutput struct {
	DroppedID string `json:"droppedId"`
}

func dropInterceptHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, DropInterceptInput) (*mcp.CallToolResult, DropInterceptOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input DropInterceptInput,
	) (*mcp.CallToolResult, DropInterceptOutput, error) {
		if input.ID == "" {
			return nil, DropInterceptOutput{}, fmt.Errorf(
				"id is required",
			)
		}

		resp, err := client.Intercept.Drop(ctx, input.ID)
		if err != nil {
			return nil, DropInterceptOutput{}, err
		}

		dropID := ""
		if resp.DropInterceptMessage.DroppedId != nil {
			dropID = *resp.DropInterceptMessage.DroppedId
		}

		return nil, DropInterceptOutput{
			DroppedID: dropID,
		}, nil
	}
}

// RegisterDropInterceptTool registers the tool
func RegisterDropInterceptTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_drop_intercept",
		Description: `Drop intercepted request (do not forward to server). The client receives no response.`,
		Annotations: writeAnn(true, true, false),
	}, dropInterceptHandler(client))
}
