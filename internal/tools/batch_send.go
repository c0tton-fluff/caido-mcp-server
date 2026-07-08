package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/c0tton-fluff/caido-mcp-server/internal/httputil"
	"github.com/c0tton-fluff/caido-mcp-server/internal/replay"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// BatchSendInput is the input for the batch_send tool.
type BatchSendInput struct {
	Requests    []BatchRequestItem `json:"requests" jsonschema:"required,Array of requests to send in parallel (max 50)"`
	Concurrency int                `json:"concurrency,omitempty" jsonschema:"Parallel session count (default 5, max 20)"`
	BodyLimit   int                `json:"bodyLimit,omitempty" jsonschema:"Response body byte limit per request (default 2000)"`
	IncludeBody *bool              `json:"includeBody,omitempty" jsonschema:"Include response body text per result (default false - bodies omitted to save tokens). The response fingerprint (title, redirect target, cookie names, word count) is always populated regardless of this setting."`
	Marker      string             `json:"marker,omitempty" jsonschema:"Optional string to search for in each response body; when set, result.reflected reports whether it was found"`
}

// BatchRequestItem is a single request in the batch.
type BatchRequestItem struct {
	Label string `json:"label" jsonschema:"required,Identifier for this request in results (e.g. owner, cross, noauth, val-1)"`
	Raw   string `json:"raw" jsonschema:"required,Full raw HTTP request including headers and body"`
	Host  string `json:"host,omitempty" jsonschema:"Target host (overrides Host header)"`
	Port  int    `json:"port,omitempty" jsonschema:"Target port (default based on TLS)"`
	TLS   *bool  `json:"tls,omitempty" jsonschema:"Use HTTPS (default true)"`
}

// BatchSendResult mirrors replay.BatchResult with the fingerprint-expansion
// fields this tool adds (Reflected). It is a standalone type rather than
// embedding replay.BatchResult so the MCP output schema stays a plain flat
// object; internal/replay's wire shape is not owned by this tool.
type BatchSendResult struct {
	Label       string                  `json:"label"`
	StatusCode  int                     `json:"statusCode,omitempty"`
	RoundtripMs int                     `json:"roundtripMs,omitempty"`
	Request     *httputil.ParsedMessage `json:"request,omitempty"`
	Response    *httputil.ParsedMessage `json:"response,omitempty"`
	Error       string                  `json:"error,omitempty"`
	Reflected   *bool                   `json:"reflected,omitempty"`
}

// BatchSendOutput is the output of the batch_send tool.
type BatchSendOutput struct {
	Results []BatchSendResult `json:"results"`
	Summary string            `json:"summary"`
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
			if err := checkRawSize(fmt.Sprintf("requests[%d]", i), r.Raw); err != nil {
				return nil, BatchSendOutput{}, err
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
		includeBody := false
		if input.IncludeBody != nil {
			includeBody = *input.IncludeBody
		}

		rawResults := replay.RunBatch(
			ctx, client, batchReqs, concurrency, bodyLimit,
		)

		// Enrich each result's fingerprint with response-only details
		// (status code, title, redirect target, cookie names, word
		// count) and, when requested, marker-reflection detection. This
		// runs here rather than inside internal/replay because RunBatch
		// already applies bodyLimit truncation before returning results,
		// so title/word-count reflect that same (possibly truncated)
		// body -- the same limitation the fingerprint itself already has.
		// Build the summary line from the same pass.
		results := make([]BatchSendResult, len(rawResults))
		ok, fail := 0, 0
		for i, r := range rawResults {
			if r.Error != "" {
				fail++
			} else {
				ok++
			}

			item := BatchSendResult{
				Label:       r.Label,
				StatusCode:  r.StatusCode,
				RoundtripMs: r.RoundtripMs,
				Request:     r.Request,
				Response:    r.Response,
				Error:       r.Error,
			}

			if r.Response != nil {
				if r.Response.Fingerprint != nil {
					httputil.PopulateResponseDetails(
						r.Response.Fingerprint, r.StatusCode,
						r.Response.Headers, []byte(r.Response.Body),
					)
				}
				if input.Marker != "" {
					reflected := strings.Contains(r.Response.Body, input.Marker)
					item.Reflected = &reflected
				}
				if !includeBody {
					r.Response.Body = ""
					r.Response.Truncated = false
				}
			}

			results[i] = item
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
		Name:        "caido_batch_send",
		Description: `Send multiple HTTP requests in parallel. Use for BAC token sweeps, parameter fuzzing, or endpoint sweeps. Max 50 per batch. Returns statusCode, headers, and a response fingerprint (title, redirect target, cookie names, word count) per request; body text is omitted by default to save tokens (set includeBody:true to include it). Pass marker to flag reflection per result.`,
		Annotations: writeAnn(false, false, true),
	}, batchSendHandler(client))
}
