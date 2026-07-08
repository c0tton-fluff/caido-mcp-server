// race_window_send wires the raceattack package into the MCP server.
//
// IMPORTANT: This tool INTENTIONALLY bypasses the Caido proxy. It dials the
// target directly from this process with raw sockets, so requests sent here do
// NOT appear in Caido history. It exists because Caido's replay polls at 50ms
// and cannot produce a tight race window. Use it for race-condition testing
// (single-packet / last-byte synchronization).
package tools

import (
	"context"
	"fmt"

	"github.com/c0tton-fluff/caido-mcp-server/internal/httputil"
	"github.com/c0tton-fluff/caido-mcp-server/internal/raceattack"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RaceRequestInput is a single raw HTTP/1.1 request to send across the barrier.
type RaceRequestInput struct {
	Label string `json:"label,omitempty" jsonschema:"Identifier for this request in results (e.g. attempt-1)"`
	Raw   string `json:"raw" jsonschema:"required,Raw HTTP/1.1 request"`
}

// RaceWindowSendInput is the input for the race_window_send tool.
type RaceWindowSendInput struct {
	Host      string             `json:"host" jsonschema:"required,Target host (no scheme)"`
	Port      int                `json:"port,omitempty" jsonschema:"Target port (default 443)"`
	Tls       *bool              `json:"tls,omitempty" jsonschema:"Use TLS (default true)"`
	Requests  []RaceRequestInput `json:"requests" jsonschema:"required,Raw requests to fire together (1..50)"`
	BodyLimit int                `json:"bodyLimit,omitempty" jsonschema:"Response body byte limit (default 4096, max 65536)"`
}

// RaceResultSummary maps a single raceattack.Result for MCP output.
type RaceResultSummary struct {
	Label      string `json:"label"`
	StatusLine string `json:"statusLine"`
	StatusCode int    `json:"statusCode"`
	Body       string `json:"body"`
	Error      string `json:"error,omitempty"`
	DurationMs int64  `json:"durationMs"`
}

// RaceWindowSendOutput is the output of the race_window_send tool.
type RaceWindowSendOutput struct {
	Results []RaceResultSummary `json:"results"`
}

// raceWindowSendHandler creates the handler for race_window_send.
func raceWindowSendHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, RaceWindowSendInput) (*mcp.CallToolResult, RaceWindowSendOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input RaceWindowSendInput,
	) (*mcp.CallToolResult, RaceWindowSendOutput, error) {
		target, reqs, err := buildRaceSend(input)
		if err != nil {
			return nil, RaceWindowSendOutput{}, err
		}
		results := raceattack.Send(ctx, target, reqs, input.BodyLimit)
		out := RaceWindowSendOutput{Results: make([]RaceResultSummary, len(results))}
		for i, r := range results {
			out.Results[i] = RaceResultSummary{
				Label:      r.Label,
				StatusLine: r.StatusLine,
				StatusCode: r.StatusCode,
				Body:       r.Body,
				Error:      r.Error,
				DurationMs: r.DurationMs,
			}
		}
		return nil, out, nil
	}
}

// buildRaceSend validates input and converts it to raceattack types.
func buildRaceSend(input RaceWindowSendInput) (raceattack.Target, []raceattack.Request, error) {
	if input.Host == "" {
		return raceattack.Target{}, nil, fmt.Errorf("host is required")
	}
	if len(input.Host) > 200 {
		return raceattack.Target{}, nil, fmt.Errorf("host exceeds 200 chars")
	}
	n := len(input.Requests)
	if n == 0 || n > 50 {
		return raceattack.Target{}, nil, fmt.Errorf(
			"requests must contain 1..50 items, got %d", n,
		)
	}
	port := input.Port
	if port == 0 {
		// Intentionally defaults to 443 regardless of TLS (TLS-unaware); does
		// not use httputil.DefaultPort so behavior is unchanged for now.
		port = 443
	}
	useTLS := true
	if input.Tls != nil {
		useTLS = *input.Tls
	}
	reqs := make([]raceattack.Request, n)
	for i, r := range input.Requests {
		if r.Raw == "" {
			return raceattack.Target{}, nil, fmt.Errorf(
				"requests[%d]: raw HTTP request is required", i,
			)
		}
		if len(r.Raw) > maxRawRequestBytes {
			return raceattack.Target{}, nil, fmt.Errorf(
				"requests[%d]: raw request exceeds 1MB limit", i,
			)
		}
		label := r.Label
		if label == "" {
			label = fmt.Sprintf("req-%d", i+1)
		}
		reqs[i] = raceattack.Request{
			Label: label,
			Raw:   httputil.NormalizeCRLF(r.Raw),
		}
	}
	return raceattack.Target{Host: input.Host, Port: port, TLS: useTLS}, reqs, nil
}

// RegisterRaceWindowSendTool registers the race_window_send tool with the
// MCP server.
func RegisterRaceWindowSendTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_race_window_send",
		Description: `Fire multiple raw HTTP/1.1 requests with a synchronized last-byte send (single-packet / race-window style) for race-condition testing. All connections are dialed and parked at a barrier; final bytes are written together after the barrier opens (best-effort simultaneity, not guaranteed sub-ms). WARNING: this BYPASSES the Caido proxy -- requests are sent via raw sockets from this process and do NOT appear in Caido history. Max 50 requests.`,
		Annotations: writeAnn(false, false, true),
	}, raceWindowSendHandler(client))
}
