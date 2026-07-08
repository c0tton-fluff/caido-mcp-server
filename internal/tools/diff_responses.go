package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/c0tton-fluff/caido-mcp-server/internal/httputil"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// DiffResponsesInput is the input for the diff_responses tool
type DiffResponsesInput struct {
	IDA string `json:"idA" jsonschema:"required,First Caido request ID"`
	IDB string `json:"idB" jsonschema:"required,Second Caido request ID"`
}

// DiffResponsesOutput is a compact structural diff between two responses
type DiffResponsesOutput struct {
	IDA           string `json:"idA"`
	IDB           string `json:"idB"`
	StatusA       int    `json:"statusA,omitempty"`
	StatusB       int    `json:"statusB,omitempty"`
	StatusChanged bool   `json:"statusChanged"`
	SizeA         int    `json:"sizeA"`
	SizeB         int    `json:"sizeB"`
	SizeDelta     int    `json:"sizeDelta"`
	BodyIdentical bool   `json:"bodyIdentical"`
	Summary       string `json:"summary"`
}

// diffSnapshot is the subset of a fetched request+response needed to compute
// a diff. hasResponse is false when the request has no response yet (e.g.
// still pending or errored) -- that is a valid state, not a fetch failure.
type diffSnapshot struct {
	id          string
	hasResponse bool
	statusCode  int
	headers     []httputil.Header
	bodySize    int
	bodyHash    uint64
}

// diffResponsesHandler creates the handler function for the diff_responses tool
func diffResponsesHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, DiffResponsesInput) (*mcp.CallToolResult, DiffResponsesOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input DiffResponsesInput,
	) (*mcp.CallToolResult, DiffResponsesOutput, error) {
		if input.IDA == "" || input.IDB == "" {
			return nil, DiffResponsesOutput{}, fmt.Errorf(
				"both idA and idB are required",
			)
		}

		snapA, err := fetchDiffSnapshot(ctx, client, input.IDA)
		if err != nil {
			return nil, DiffResponsesOutput{}, err
		}
		snapB, err := fetchDiffSnapshot(ctx, client, input.IDB)
		if err != nil {
			return nil, DiffResponsesOutput{}, err
		}

		return nil, buildDiffOutput(snapA, snapB), nil
	}
}

// fetchDiffSnapshot fetches a request+response by Caido-native ID and
// reduces it to the fields needed for diffing. It reuses the same fetch
// path as caido_get_request (client.Requests.Get) and the same body parsing
// primitive (httputil.ParseBase64), requesting the full untruncated body
// (bodyLimit=0) so bodyHash/bodySize reflect the real response, even though
// the full body is never included in the tool output.
func fetchDiffSnapshot(
	ctx context.Context, client *caido.Client, id string,
) (diffSnapshot, error) {
	resp, err := client.Requests.Get(ctx, id)
	if err != nil {
		return diffSnapshot{}, fmt.Errorf(
			"failed to get request %s: %w", id, err,
		)
	}

	r := resp.Request
	if r == nil {
		return diffSnapshot{}, fmt.Errorf("request %s not found", id)
	}

	if r.Response == nil {
		return diffSnapshot{id: r.Id}, nil
	}

	snap := diffSnapshot{
		id:          r.Id,
		hasResponse: true,
		statusCode:  r.Response.StatusCode,
	}

	parsed := httputil.ParseBase64(r.Response.Raw, true, true, 0, 0)
	if parsed != nil {
		snap.headers = parsed.Headers
		snap.bodySize = parsed.BodySize
		snap.bodyHash = httputil.HashBody([]byte(parsed.Body))
	}
	return snap, nil
}

// buildDiffOutput computes the compact diff summary from two snapshots.
func buildDiffOutput(a, b diffSnapshot) DiffResponsesOutput {
	output := DiffResponsesOutput{
		IDA:           a.id,
		IDB:           b.id,
		StatusA:       a.statusCode,
		StatusB:       b.statusCode,
		StatusChanged: a.statusCode != b.statusCode,
		SizeA:         a.bodySize,
		SizeB:         b.bodySize,
		SizeDelta:     b.bodySize - a.bodySize,
		BodyIdentical: a.hasResponse && b.hasResponse &&
			a.bodyHash == b.bodyHash && a.bodySize == b.bodySize,
	}
	output.Summary = diffSummary(output, a, b)
	return output
}

// diffSummary renders a brief human-readable summary: status change, size
// change, and a compact header add/remove/change count. Full header values
// and bodies are intentionally never included to keep the tool output
// token-efficient.
func diffSummary(output DiffResponsesOutput, a, b diffSnapshot) string {
	if !a.hasResponse && !b.hasResponse {
		return "neither request has a response"
	}
	if !a.hasResponse {
		return "idA has no response"
	}
	if !b.hasResponse {
		return "idB has no response"
	}

	var parts []string
	if output.StatusChanged {
		parts = append(parts, fmt.Sprintf(
			"status %d -> %d", a.statusCode, b.statusCode,
		))
	}
	if output.SizeDelta != 0 {
		sign := "+"
		if output.SizeDelta < 0 {
			sign = ""
		}
		parts = append(parts, fmt.Sprintf(
			"body %s%d bytes", sign, output.SizeDelta,
		))
	}
	if hd := headerDiffSummary(a.headers, b.headers); hd != "" {
		parts = append(parts, hd)
	}

	if len(parts) == 0 {
		return "identical"
	}
	return strings.Join(parts, "; ")
}

// headerDiffSummary compares header sets by name (case-insensitive) and
// returns a compact "headers +added -removed ~changed" count, or "" if the
// header sets are identical. Values already reflect any sensitive-header
// redaction applied by httputil.ParseBase64, so this never surfaces secrets.
func headerDiffSummary(a, b []httputil.Header) string {
	toMap := func(hs []httputil.Header) map[string]string {
		m := make(map[string]string, len(hs))
		for _, h := range hs {
			m[strings.ToLower(h.Name)] = h.Value
		}
		return m
	}
	ma, mb := toMap(a), toMap(b)

	var added, removed, changed int
	for name, valB := range mb {
		if valA, ok := ma[name]; !ok {
			added++
		} else if valA != valB {
			changed++
		}
	}
	for name := range ma {
		if _, ok := mb[name]; !ok {
			removed++
		}
	}

	if added == 0 && removed == 0 && changed == 0 {
		return ""
	}
	return fmt.Sprintf("headers +%d -%d ~%d", added, removed, changed)
}

// RegisterDiffResponsesTool registers the tool with the MCP server
func RegisterDiffResponsesTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_diff_responses",
		Description: `Compare two responses by Caido request ID (idA, idB). Returns status/size change flags and a compact body/header diff summary. Does not dump full bodies.`,
		Annotations: readOnlyAnn(),
	}, diffResponsesHandler(client))
}
