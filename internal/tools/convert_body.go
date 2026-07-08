package tools

import (
	"context"
	"fmt"

	"github.com/c0tton-fluff/caido-mcp-server/internal/httputil"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ConvertBodyInput is the input for the convert_body tool.
type ConvertBodyInput struct {
	Body string `json:"body" jsonschema:"required,Body content to convert"`
	From string `json:"from" jsonschema:"required,Source format: json, form, xml, or multipart"`
	To   string `json:"to" jsonschema:"required,Target format: json, form, xml, or multipart"`
}

// ConvertBodyOutput is the output of the convert_body tool.
type ConvertBodyOutput struct {
	Body        string `json:"body"`
	ContentType string `json:"contentType"`
}

// convertBodyHandler creates the handler function. The Caido client is unused
// (pure transformation) but kept in the signature for registration
// consistency with the other tools.
func convertBodyHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, ConvertBodyInput) (*mcp.CallToolResult, ConvertBodyOutput, error) {
	_ = client
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input ConvertBodyInput,
	) (*mcp.CallToolResult, ConvertBodyOutput, error) {
		if len(input.Body) > maxRawRequestBytes {
			return nil, ConvertBodyOutput{}, fmt.Errorf(
				"body exceeds max length of %d bytes", maxRawRequestBytes)
		}

		from := httputil.BodyFormat(input.From)
		if !httputil.IsKnownFormat(from) {
			return nil, ConvertBodyOutput{}, fmt.Errorf(
				"unknown 'from' format %q (want json, form, xml, or multipart)",
				input.From)
		}
		to := httputil.BodyFormat(input.To)
		if !httputil.IsKnownFormat(to) {
			return nil, ConvertBodyOutput{}, fmt.Errorf(
				"unknown 'to' format %q (want json, form, xml, or multipart)",
				input.To)
		}

		converted, contentType, err := httputil.ConvertBody(input.Body, from, to)
		if err != nil {
			return nil, ConvertBodyOutput{}, err
		}

		return nil, ConvertBodyOutput{
			Body:        converted,
			ContentType: contentType,
		}, nil
	}
}

// RegisterConvertBodyTool registers the tool with the MCP server.
func RegisterConvertBodyTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "caido_convert_body",
		Description: `Convert a request/response body between formats. ` +
			`Formats: json, form (x-www-form-urlencoded), xml, multipart ` +
			`(form-data). Params: body, from, to. ` +
			`Returns the converted body and matching Content-Type. ` +
			`JSON<->form supports flat objects losslessly (nested uses ` +
			`bracket notation a[b]=c). multipart handles flat string fields ` +
			`only (no files).`,
	}, convertBodyHandler(client))
}
