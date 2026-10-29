package tools

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/httputil"
	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/replay"
	caido "github.com/caido-community/sdk-go"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	maxPipelineRequests     = 50
	defaultPipelineBody     = 256
	maxPipelineBody         = 4096
	defaultPipelineTimeoutS = 30
	maxPipelineTimeoutS     = 120
)

// RacePipelineInput is the input for the race_pipeline tool.
type RacePipelineInput struct {
	Host           string             `json:"host" jsonschema:"required,Target host (no scheme)"`
	Port           int                `json:"port,omitempty" jsonschema:"Target port (default 443 with TLS, 80 without)"`
	TLS            *bool              `json:"tls,omitempty" jsonschema:"Use TLS (default true)"`
	Requests       []RaceRequestInput `json:"requests" jsonschema:"required,Raw HTTP/1.1 requests to run together (1..50). Repeat one request N times to race it."`
	Strategy       string             `json:"strategy,omitempty" jsonschema:"last-byte (default): hold each final byte and release together; single-packet: one HTTP/2 packet releases every request (needs Caido's v2 network stack); sequential: one after another"`
	ConvertToHTTP2 *bool              `json:"convertToHttp2,omitempty" jsonschema:"single-packet only: send the HTTP/1 requests over one HTTP/2 connection (default true)"`
	FireOnPartial  bool               `json:"fireOnPartial,omitempty" jsonschema:"single-packet/last-byte: still release the requests that prepared when some failed (default false: abort the run)"`
	AbortOnFailure bool               `json:"abortOnFailure,omitempty" jsonschema:"sequential only: stop at the first failed request"`
	CollectionID   string             `json:"collectionId,omitempty" jsonschema:"File the pipeline session in this replay collection"`
	Name           string             `json:"name,omitempty" jsonschema:"Title for the pipeline session (e.g. Race - coupon redeem)"`
	BodyLimit      int                `json:"bodyLimit,omitempty" jsonschema:"Per-response body byte limit (default 256, max 4096)"`
	TimeoutSec     int                `json:"timeoutSec,omitempty" jsonschema:"Seconds to wait for every response (default 30, max 120)"`
}

// RacePipelineResult is one request's outcome.
type RacePipelineResult struct {
	Label       string `json:"label"`
	RequestID   string `json:"requestId,omitempty"`
	StatusCode  int    `json:"statusCode,omitempty"`
	RoundtripMs int    `json:"roundtripMs,omitempty"`
	Length      int    `json:"length,omitempty"`
	Body        string `json:"body,omitempty"`
	Truncated   bool   `json:"truncated,omitempty"`
	Error       string `json:"error,omitempty"`
	Pending     bool   `json:"pending,omitempty"`
}

// RacePipelineOutput is the output of the race_pipeline tool.
type RacePipelineOutput struct {
	SessionID string `json:"sessionId"`
	EntryID   string `json:"entryId"`
	Strategy  string `json:"strategy"`
	// Complete is false when the timeout hit first; pending results have
	// no response yet and the run is still visible in Caido Replay.
	Complete bool   `json:"complete"`
	Error    string `json:"error,omitempty"`
	// StatusCounts tallies responses by status code ("error" and "pending"
	// for the rest): the quick read on whether the race let more than one
	// request through.
	StatusCounts map[string]int       `json:"statusCounts"`
	Results      []RacePipelineResult `json:"results"`
}

// buildRacePipeline validates input and converts it to replay types.
func buildRacePipeline(input RacePipelineInput) ([]replay.PipelineRequest, replay.PipelineOptions, error) {
	var opts replay.PipelineOptions
	if input.Host == "" {
		return nil, opts, fmt.Errorf("host is required")
	}
	if len(input.Host) > 200 {
		return nil, opts, fmt.Errorf("host exceeds 200 chars")
	}
	n := len(input.Requests)
	if n == 0 || n > maxPipelineRequests {
		return nil, opts, fmt.Errorf(
			"requests must contain 1..%d items, got %d", maxPipelineRequests, n,
		)
	}
	strategy := input.Strategy
	if strategy == "" {
		strategy = replay.StrategyLastByte
	}
	if !slices.Contains(replay.PipelineStrategies, strategy) {
		return nil, opts, fmt.Errorf(
			"strategy must be one of %v, got %q", replay.PipelineStrategies, strategy,
		)
	}
	useTLS := true
	if input.TLS != nil {
		useTLS = *input.TLS
	}
	port := input.Port
	if port == 0 {
		port = httputil.DefaultPort(useTLS)
	}
	conn := caido.ReplayConnection{Host: input.Host, Port: port, IsTLS: useTLS}
	reqs := make([]replay.PipelineRequest, n)
	for i, r := range input.Requests {
		if r.Raw == "" {
			return nil, opts, fmt.Errorf("requests[%d]: raw HTTP request is required", i)
		}
		if len(r.Raw) > maxRawRequestBytes {
			return nil, opts, fmt.Errorf("requests[%d]: raw request exceeds 1MB limit", i)
		}
		reqs[i] = replay.PipelineRequest{Raw: httputil.NormalizeCRLF(r.Raw), Conn: conn}
	}
	convert := true
	if input.ConvertToHTTP2 != nil {
		convert = *input.ConvertToHTTP2
	}
	timeout := input.TimeoutSec
	if timeout <= 0 {
		timeout = defaultPipelineTimeoutS
	}
	timeout = min(timeout, maxPipelineTimeoutS)
	opts = replay.PipelineOptions{
		Strategy:       strategy,
		ConvertToHTTP2: convert,
		FireOnPartial:  input.FireOnPartial,
		AbortOnFailure: input.AbortOnFailure,
		CollectionID:   input.CollectionID,
		Name:           input.Name,
		Timeout:        time.Duration(timeout) * time.Second,
	}
	return reqs, opts, nil
}

// racePipelineOutput maps a PipelineRun to tool output. Results are matched
// to input labels by position; Caido keeps the pipeline's request order.
func racePipelineOutput(
	run *replay.PipelineRun, input RacePipelineInput, strategy string,
) RacePipelineOutput {
	bodyLimit := input.BodyLimit
	if bodyLimit <= 0 {
		bodyLimit = defaultPipelineBody
	}
	bodyLimit = min(bodyLimit, maxPipelineBody)

	out := RacePipelineOutput{
		SessionID:    run.SessionID,
		EntryID:      run.EntryID,
		Strategy:     strategy,
		Complete:     run.Complete,
		Error:        run.Error,
		StatusCounts: map[string]int{},
		Results:      make([]RacePipelineResult, len(input.Requests)),
	}
	for i, r := range input.Requests {
		label := r.Label
		if label == "" {
			label = fmt.Sprintf("req-%d", i+1)
		}
		res := RacePipelineResult{Label: label}
		switch {
		case i >= len(run.Results):
			res.Pending = true
		default:
			pr := run.Results[i]
			res.RequestID = pr.RequestID
			res.Error = pr.Error
			if pr.HasResponse {
				res.StatusCode = pr.StatusCode
				res.RoundtripMs = pr.RoundtripMs
				res.Length = pr.Length
				if msg := httputil.ParseBase64(pr.ResponseRaw, false, true, 0, bodyLimit); msg != nil {
					res.Body = msg.Body
					res.Truncated = msg.Truncated
				}
			} else if pr.Error == "" {
				res.Pending = true
			}
		}
		switch {
		case res.StatusCode != 0:
			out.StatusCounts[strconv.Itoa(res.StatusCode)]++
		case res.Error != "":
			out.StatusCounts["error"]++
		default:
			out.StatusCounts["pending"]++
		}
		out.Results[i] = res
	}
	return out
}

func racePipelineHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, RacePipelineInput) (*mcp.CallToolResult, RacePipelineOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input RacePipelineInput,
	) (*mcp.CallToolResult, RacePipelineOutput, error) {
		reqs, opts, err := buildRacePipeline(input)
		if err != nil {
			return nil, RacePipelineOutput{}, err
		}
		run, err := replay.RunPipeline(ctx, client, reqs, opts)
		if err != nil {
			return nil, RacePipelineOutput{}, err
		}
		return nil, racePipelineOutput(run, input, opts.Strategy), nil
	}
}

// RegisterRacePipelineTool registers the race_pipeline tool.
func RegisterRacePipelineTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:  "caido_race_pipeline",
		Title: "Race Requests (Replay Pipeline)",
		Description: `Race-condition test through Caido itself (needs Caido 0.59+): creates a Replay Pipeline session holding the requests and runs them with a synchronized strategy. ` +
			`Requests go through the proxy and appear in Caido history and Replay, unlike caido_race_window_send. ` +
			`Returns per-request status/timing/body and statusCounts (e.g. {"200":2,"409":18} means two requests won the race). ` +
			`single-packet needs Caido's experimental v2 network stack (Settings > Network > HTTP Stack); without it each request reports that error.`,
		InputSchema: schemaFor[RacePipelineInput](func(s *jsonschema.Schema) {
			if p := prop(s, "strategy"); p != nil {
				p.Enum = []any{
					replay.StrategyLastByte, replay.StrategySinglePacket, replay.StrategySequential,
				}
			}
			if p := prop(s, "host"); p != nil {
				p.MaxLength = intPtr(200)
			}
			if p := prop(s, "requests"); p != nil {
				p.MinItems = intPtr(1)
				p.MaxItems = intPtr(maxPipelineRequests)
				if p.Items != nil {
					if raw := prop(p.Items, "raw"); raw != nil {
						raw.MaxLength = intPtr(maxRawRequestBytes)
					}
				}
			}
		}),
		Annotations: writeAnn(false, false, true),
	}, racePipelineHandler(client))
}
