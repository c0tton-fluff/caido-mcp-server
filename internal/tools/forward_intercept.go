package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	gen "github.com/caido-community/sdk-go/graphql"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ForwardInterceptInput is the input for the tool
type ForwardInterceptInput struct {
	ID  string `json:"id" jsonschema:"required,Intercept entry ID to forward"`
	Raw string `json:"raw,omitempty" jsonschema:"Modified raw HTTP request (base64-encoded). Omit to forward unmodified."`
}

// ForwardInterceptOutput is the output
type ForwardInterceptOutput struct {
	ForwardedID string `json:"forwardedId"`
}

func forwardInterceptHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, ForwardInterceptInput) (*mcp.CallToolResult, ForwardInterceptOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input ForwardInterceptInput,
	) (*mcp.CallToolResult, ForwardInterceptOutput, error) {
		if input.ID == "" {
			return nil, ForwardInterceptOutput{}, fmt.Errorf(
				"id is required",
			)
		}
		if len(input.Raw) > 1048576 {
			return nil, ForwardInterceptOutput{}, fmt.Errorf(
				"raw request exceeds max length of 1MB",
			)
		}

		var fwdInput *gen.ForwardInterceptMessageInput
		if input.Raw != "" {
			fwdInput = &gen.ForwardInterceptMessageInput{
				Request: &gen.ForwardInterceptRequestMessageInput{
					UpdateRaw:           input.Raw,
					UpdateContentLength: true,
				},
			}
		}

		resp, err := client.Intercept.Forward(
			ctx, input.ID, fwdInput,
		)
		if err != nil {
			return nil, ForwardInterceptOutput{}, err
		}

		fwdID := ""
		if resp.ForwardInterceptMessage.ForwardedId != nil {
			fwdID = *resp.ForwardInterceptMessage.ForwardedId
		}

		return nil, ForwardInterceptOutput{
			ForwardedID: fwdID,
		}, nil
	}
}

// RegisterForwardInterceptTool registers the tool
func RegisterForwardInterceptTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_forward_intercept",
		Description: `Forward intercepted request. Optionally modify with base64-encoded raw HTTP request. Params: id (required), raw (optional).`,
	}, forwardInterceptHandler(client))
}
