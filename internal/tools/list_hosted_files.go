package tools

import (
	"context"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ListHostedFilesInput struct{}

type HostedFileSummary struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Path   string `json:"path"`
	Size   int    `json:"size"`
	Status string `json:"status"`
}

type ListHostedFilesOutput struct {
	Files []HostedFileSummary `json:"files"`
}

func listHostedFilesHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, ListHostedFilesInput) (*mcp.CallToolResult, ListHostedFilesOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input ListHostedFilesInput,
	) (*mcp.CallToolResult, ListHostedFilesOutput, error) {
		resp, err := client.HostedFiles.List(ctx)
		if err != nil {
			return nil, ListHostedFilesOutput{}, err
		}

		output := ListHostedFilesOutput{
			Files: make([]HostedFileSummary, 0, len(resp.HostedFiles)),
		}
		for _, f := range resp.HostedFiles {
			output.Files = append(output.Files, HostedFileSummary{
				ID:     f.Id,
				Name:   f.Name,
				Path:   f.Path,
				Size:   f.Size,
				Status: string(f.Status),
			})
		}

		return nil, output, nil
	}
}

func RegisterListHostedFilesTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_list_hosted_files",
		Description: `List hosted files available in Caido for serving payloads.`,
		InputSchema: map[string]any{"type": "object"},
	}, listHostedFilesHandler(client))
}
