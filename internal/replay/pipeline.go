package replay

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"time"

	gql "github.com/Khan/genqlient/graphql"
	caido "github.com/caido-community/sdk-go"
)

// Replay Pipelines (Caido 0.58+, single-packet attack since 0.59) group
// requests into one run that Caido itself sends with a chosen strategy.
// Unlike raceattack, the requests go through the proxy and land in history.
// sdk-go has no pipeline operations, so these are hand-written; inputs are
// maps because genqlient drops @oneOf members incorrectly (see
// create_tamper_rule.go).

// Pipeline strategies accepted by RunPipeline. last-byte is the default
// because it works on a stock Caido; single-packet needs the experimental
// v2 (HTTP/2) network stack, and without it every request fails with
// "single packet attack requires the v2 network stack".
const (
	StrategyLastByte     = "last-byte"
	StrategySinglePacket = "single-packet"
	StrategySequential   = "sequential"
)

// PipelineStrategies lists the accepted strategies, default first.
var PipelineStrategies = []string{
	StrategyLastByte, StrategySinglePacket, StrategySequential,
}

const (
	pipelinePollInit = 100 * time.Millisecond
	pipelinePollMax  = time.Second
)

const createPipelineMutation = `mutation CreateReplayPipelineSession($input: CreateReplayPipelineSessionInput!) {
  createReplayPipelineHttpSession(input: $input) {
    session { id }
    error {
      __typename
      ... on UserError { code }
      ... on PermissionDeniedUserError { permissionReason: reason }
      ... on CloudUserError { cloudReason: reason }
    }
  }
}`

const startPipelineMutation = `mutation StartReplayPipelineTask($sessionId: ID!) {
  startReplayTask(sessionId: $sessionId) {
    task { id replayEntry { id } }
    error { __typename ... on UserError { code } }
  }
}`

const getPipelineEntryQuery = `query GetReplayPipelineEntry($id: ID!) {
  replayEntry(id: $id, sessionKind: HTTP_PIPELINE) {
    id
    error
    ... on ReplayEntryHttpPipeline {
      httpEntries {
        id
        error
        request {
          id
          response { statusCode roundtripTime length raw }
        }
      }
    }
  }
}`

// PipelineRequest is one raw request in a pipeline run.
type PipelineRequest struct {
	Raw  string // plain text, CRLF line endings
	Conn caido.ReplayConnection
}

// PipelineOptions configures a pipeline run.
type PipelineOptions struct {
	Strategy string // one of PipelineStrategies; empty = last-byte
	// ConvertToHTTP2 applies to single-packet: send the HTTP/1 requests over
	// one HTTP/2 connection (the classic single-packet attack).
	ConvertToHTTP2 bool
	// FireOnPartial applies to single-packet and last-byte: still release
	// the requests that prepared when some failed. Default aborts the run.
	FireOnPartial bool
	// AbortOnFailure applies to sequential: stop at the first failure.
	AbortOnFailure bool
	CollectionID   string
	Name           string
	// Timeout bounds how long RunPipeline waits for results after start.
	Timeout time.Duration
}

// PipelineResult is the outcome of one request in a run.
type PipelineResult struct {
	HTTPEntryID string
	RequestID   string
	Error       string
	HasResponse bool
	StatusCode  int
	RoundtripMs int
	Length      int
	ResponseRaw string // base64
}

// PipelineRun is the outcome of RunPipeline. Complete is false when the
// timeout hit before every request resolved; Results then holds what
// arrived so far and EntryID can be polled again via PollPipeline.
type PipelineRun struct {
	SessionID string
	EntryID   string
	Error     string
	Complete  bool
	Results   []PipelineResult
}

// strategyInput builds the PipelineStrategyInput @oneOf value.
func strategyInput(o PipelineOptions) (map[string]any, error) {
	failure := "ABORT_ON_PARTIAL"
	if o.FireOnPartial {
		failure = "FIRE_ON_PARTIAL"
	}
	switch o.Strategy {
	case StrategySinglePacket:
		return map[string]any{"singlePacketAttack": map[string]any{
			"convertToHttp2": o.ConvertToHTTP2, "failureBehavior": failure,
		}}, nil
	case "", StrategyLastByte:
		return map[string]any{"lastByteSynchronization": map[string]any{
			"failureBehavior": failure,
		}}, nil
	case StrategySequential:
		return map[string]any{"sequential": map[string]any{
			"abortOnFailure": o.AbortOnFailure,
		}}, nil
	}
	return nil, fmt.Errorf(
		"unknown strategy %q (want one of %v)", o.Strategy, PipelineStrategies,
	)
}

type userErrorRef struct {
	Typename         string `json:"__typename"`
	Code             string `json:"code"`
	PermissionReason string `json:"permissionReason"`
	CloudReason      string `json:"cloudReason"`
}

func (e *userErrorRef) String() string {
	msg := e.Typename
	if e.Code != "" {
		msg += ": " + e.Code
	}
	if reason := e.PermissionReason + e.CloudReason; reason != "" {
		msg += " (" + reason + ")"
	}
	if e.PermissionReason == "ENTITLEMENT" {
		// Seen live on 0.59.0: the plan caps how many pipeline sessions
		// may exist; deleting one let the next create succeed.
		msg += "; the Caido plan's limit on pipeline sessions is reached, " +
			"delete an old pipeline session (caido_list_replay_sessions, " +
			"kind pipeline) or upgrade the plan"
	}
	return msg
}

// RunPipeline creates a pipeline session holding reqs, optionally files and
// names it, starts it, and waits up to opts.Timeout for every request to
// resolve.
func RunPipeline(
	ctx context.Context, client *caido.Client,
	reqs []PipelineRequest, opts PipelineOptions,
) (*PipelineRun, error) {
	if len(reqs) == 0 {
		return nil, fmt.Errorf("pipeline needs at least one request")
	}
	strategy, err := strategyInput(opts)
	if err != nil {
		return nil, err
	}
	sources := make([]map[string]any, len(reqs))
	for i, r := range reqs {
		sources[i] = map[string]any{"raw": map[string]any{
			"raw": base64.StdEncoding.EncodeToString([]byte(r.Raw)),
			"connectionInfo": map[string]any{
				"host": r.Conn.Host, "port": r.Conn.Port, "isTLS": r.Conn.IsTLS,
			},
		}}
	}
	input := map[string]any{
		"requestSources": sources,
		"settings":       map[string]any{"strategy": strategy},
	}
	if opts.CollectionID != "" {
		input["collectionId"] = opts.CollectionID
	}

	var created struct {
		CreateReplayPipelineHttpSession struct {
			Session *struct {
				ID string `json:"id"`
			} `json:"session"`
			Error *userErrorRef `json:"error"`
		} `json:"createReplayPipelineHttpSession"`
	}
	if err := client.GraphQL.MakeRequest(ctx, &gql.Request{
		OpName:    "CreateReplayPipelineSession",
		Query:     createPipelineMutation,
		Variables: map[string]any{"input": input},
	}, &gql.Response{Data: &created}); err != nil {
		return nil, fmt.Errorf(
			"create pipeline session (needs Caido 0.59+): %w", err,
		)
	}
	payload := created.CreateReplayPipelineHttpSession
	if payload.Error != nil {
		return nil, fmt.Errorf("create pipeline session: %s", payload.Error)
	}
	if payload.Session == nil {
		return nil, fmt.Errorf("create pipeline session returned no session")
	}
	sessionID := payload.Session.ID

	if opts.Name != "" {
		if _, _, err := RenameSession(ctx, client, sessionID, opts.Name); err != nil {
			// Nothing has been sent yet; do not leave an unnamed session.
			if _, delErr := client.Replay.DeleteSessions(
				ctx, []string{sessionID},
			); delErr != nil {
				fmt.Fprintf(
					os.Stderr,
					"replay: failed to delete orphaned session %s: %v\n",
					sessionID, delErr,
				)
			}
			return nil, fmt.Errorf("rename session %s: %w", sessionID, err)
		}
	}

	var started struct {
		StartReplayTask struct {
			Task *struct {
				ReplayEntry struct {
					ID string `json:"id"`
				} `json:"replayEntry"`
			} `json:"task"`
			Error *userErrorRef `json:"error"`
		} `json:"startReplayTask"`
	}
	if err := client.GraphQL.MakeRequest(ctx, &gql.Request{
		OpName:    "StartReplayPipelineTask",
		Query:     startPipelineMutation,
		Variables: map[string]any{"sessionId": sessionID},
	}, &gql.Response{Data: &started}); err != nil {
		return nil, fmt.Errorf("start pipeline session %s: %w", sessionID, err)
	}
	st := started.StartReplayTask
	if st.Error != nil {
		return nil, fmt.Errorf("start pipeline session %s: %s", sessionID, st.Error)
	}
	if st.Task == nil || st.Task.ReplayEntry.ID == "" {
		return nil, fmt.Errorf("start pipeline session %s returned no entry", sessionID)
	}

	run, err := PollPipeline(ctx, client, st.Task.ReplayEntry.ID, len(reqs), opts.Timeout)
	if run != nil {
		run.SessionID = sessionID
	}
	return run, err
}

type pipelineEntryResp struct {
	ReplayEntry *struct {
		ID          string  `json:"id"`
		Error       *string `json:"error"`
		HTTPEntries []struct {
			ID      string  `json:"id"`
			Error   *string `json:"error"`
			Request *struct {
				ID       string `json:"id"`
				Response *struct {
					StatusCode    int    `json:"statusCode"`
					RoundtripTime int    `json:"roundtripTime"`
					Length        int    `json:"length"`
					Raw           string `json:"raw"`
				} `json:"response"`
			} `json:"request"`
		} `json:"httpEntries"`
	} `json:"replayEntry"`
}

// PollPipeline reads a pipeline entry until want requests have resolved
// (response or error), the entry reports an error, or timeout passes. It
// returns the latest snapshot either way; Complete says which.
func PollPipeline(
	ctx context.Context, client *caido.Client,
	entryID string, want int, timeout time.Duration,
) (*PipelineRun, error) {
	deadline := time.Now().Add(timeout)
	interval := pipelinePollInit
	for {
		var data pipelineEntryResp
		if err := client.GraphQL.MakeRequest(ctx, &gql.Request{
			OpName:    "GetReplayPipelineEntry",
			Query:     getPipelineEntryQuery,
			Variables: map[string]any{"id": entryID},
		}, &gql.Response{Data: &data}); err != nil {
			return nil, fmt.Errorf("read pipeline entry %s: %w", entryID, err)
		}
		if data.ReplayEntry == nil {
			return nil, fmt.Errorf("pipeline entry %s not found", entryID)
		}
		run := snapshot(entryID, data, want)
		if run.Complete || !time.Now().Before(deadline) {
			return run, nil
		}
		select {
		case <-ctx.Done():
			return run, nil
		case <-time.After(interval):
		}
		interval = min(interval*2, pipelinePollMax)
	}
}

// snapshot converts a pipeline entry read into a PipelineRun.
func snapshot(entryID string, data pipelineEntryResp, want int) *PipelineRun {
	e := data.ReplayEntry
	run := &PipelineRun{EntryID: entryID}
	if e.Error != nil {
		run.Error = *e.Error
	}
	resolved := 0
	for _, h := range e.HTTPEntries {
		r := PipelineResult{HTTPEntryID: h.ID}
		if h.Error != nil {
			r.Error = *h.Error
		}
		if h.Request != nil {
			r.RequestID = h.Request.ID
			if resp := h.Request.Response; resp != nil {
				r.HasResponse = true
				r.StatusCode = resp.StatusCode
				r.RoundtripMs = resp.RoundtripTime
				r.Length = resp.Length
				r.ResponseRaw = resp.Raw
			}
		}
		if r.HasResponse || r.Error != "" {
			resolved++
		}
		run.Results = append(run.Results, r)
	}
	run.Complete = run.Error != "" || (resolved >= want && len(e.HTTPEntries) >= want)
	return run
}
