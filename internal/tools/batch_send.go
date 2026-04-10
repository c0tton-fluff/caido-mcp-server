package tools

import (
	"context"
	"fmt"

	"github.com/c0tton-fluff/caido-mcp-server/internal/replay"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// BatchSendInput is the input for the batch_send tool.
type BatchSendInput struct {
	Requests    []BatchRequestItem `json:"requests" jsonschema:"required,Array of requests to send in parallel (max 50)"`
	Concurrency int                `json:"concurrency,omitempty" jsonschema:"Parallel session count (default 5, max 20)"`
	BodyLimit   int                `json:"bodyLimit,omitempty" jsonschema:"Response body byte limit per request (default 2000)"`
}

// BatchRequestItem is a single request in the batch.
type BatchRequestItem struct {
	Label string `json:"label" jsonschema:"required,Identifier for this request in results (e.g. owner, cross, noauth, val-1)"`
	Raw   string `json:"raw" jsonschema:"required,Full raw HTTP request including headers and body"`
	Host  string `json:"host,omitempty" jsonschema:"Target host (overrides Host header)"`
	Port  int    `json:"port,omitempty" jsonschema:"Target port (default based on TLS)"`
	TLS   *bool  `json:"tls,omitempty" jsonschema:"Use HTTPS (default true)"`
}

// BatchSendOutput is the output of the batch_send tool.
type BatchSendOutput struct {
	Results []replay.BatchResult `json:"results"`
	Summary string               `json:"summary"`
}

// batchSendHandler creates the handler function for batch_send.
func batchSendHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, BatchSendInput) (*mcp.CallToolResult, BatchSendOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input BatchSendInput,
	) (*mcp.CallToolResult, BatchSendOutput, error) {
		n := len(input.Requests)
		if n == 0 {
			return nil, BatchSendOutput{}, fmt.Errorf(
				"requests array is required and must not be empty",
			)
		}
		if n > 50 {
			return nil, BatchSendOutput{}, fmt.Errorf(
				"max 50 requests per batch, got %d", n,
			)
		}

		// Validate each request.
		for i, r := range input.Requests {
			if r.Raw == "" {
				return nil, BatchSendOutput{}, fmt.Errorf(
					"requests[%d]: raw HTTP request is required", i,
				)
			}
			if len(r.Raw) > 1048576 {
				return nil, BatchSendOutput{}, fmt.Errorf(
					"requests[%d]: raw request exceeds 1MB limit", i,
				)
			}
			if r.Label == "" {
				input.Requests[i].Label = fmt.Sprintf("req-%d", i+1)
			}
		}

		// Convert to internal batch request format.
		batchReqs := make([]replay.BatchRequest, n)
		for i, r := range input.Requests {
			batchReqs[i] = replay.BatchRequest{
				Label: r.Label,
				Raw:   r.Raw,
				Host:  r.Host,
				Port:  r.Port,
				TLS:   r.TLS,
			}
		}

		concurrency := input.Concurrency
		if concurrency == 0 {
			concurrency = 5
		}
		bodyLimit := input.BodyLimit
		if bodyLimit == 0 {
			bodyLimit = 2000
		}

		results := replay.RunBatch(
			ctx, client, batchReqs, concurrency, bodyLimit,
		)

		// Build summary line.
		ok, fail := 0, 0
		for _, r := range results {
			if r.Error != "" {
				fail++
			} else {
				ok++
			}
		}
		summary := fmt.Sprintf(
			"%d/%d succeeded", ok, n,
		)
		if fail > 0 {
			summary += fmt.Sprintf(", %d failed", fail)
		}

		return nil, BatchSendOutput{
			Results: results,
			Summary: summary,
		}, nil
	}
}

// RegisterBatchSendTool registers the batch_send tool with the
// MCP server.
func RegisterBatchSendTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "caido_batch_send",
		Description: `Send multiple HTTP requests in parallel and return all responses. ` +
			`Creates a session pool, dispatches requests concurrently, polls for results. ` +
			`Use for: BAC token sweeps (same endpoint, different auth), parameter fuzzing ` +
			`(same endpoint, different values), endpoint sweeps (different URLs, same auth). ` +
			`Max 50 requests per batch. Each request needs a label and raw HTTP. ` +
			`Returns results array with statusCode, headers, body per request.`,
	}, batchSendHandler(client))
}
