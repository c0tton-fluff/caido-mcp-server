package replay

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/c0tton-fluff/caido-mcp-server/internal/httputil"
	caido "github.com/caido-community/sdk-go"
	gen "github.com/caido-community/sdk-go/graphql"
)

// BatchRequest is a single request in a batch.
type BatchRequest struct {
	Label string
	Raw   string // raw HTTP, will be CRLF-normalized
	Host  string // override Host header
	Port  int    // override port
	TLS   *bool  // override TLS (default true)
}

// BatchResult is the result of a single request in a batch.
type BatchResult struct {
	Label       string                  `json:"label"`
	StatusCode  int                     `json:"statusCode,omitempty"`
	RoundtripMs int                     `json:"roundtripMs,omitempty"`
	Request     *httputil.ParsedMessage `json:"request,omitempty"`
	Response    *httputil.ParsedMessage `json:"response,omitempty"`
	Error       string                  `json:"error,omitempty"`
}

// RunBatch sends N requests in parallel through Caido's Replay API.
// It creates a session pool, dispatches each request to its own
// session, polls for results, and returns them in order.
func RunBatch(
	ctx context.Context,
	client *caido.Client,
	requests []BatchRequest,
	concurrency int,
	bodyLimit int,
) []BatchResult {
	if concurrency < 1 {
		concurrency = 5
	}
	if concurrency > 20 {
		concurrency = 20
	}
	if bodyLimit <= 0 {
		bodyLimit = httputil.DefaultBodyLimit
	}

	n := len(requests)
	if n == 0 {
		return nil
	}

	// Cap concurrency to request count.
	poolSize := min(concurrency, n)

	// Create session pool. If this fails, return all errors.
	pool, err := NewSessionPool(ctx, client, poolSize)
	if err != nil {
		results := make([]BatchResult, n)
		for i := range results {
			results[i] = BatchResult{
				Label: requests[i].Label,
				Error: fmt.Sprintf("session pool: %v", err),
			}
		}
		return results
	}

	results := make([]BatchResult, n)
	var wg sync.WaitGroup

	for i, req := range requests {
		wg.Add(1)
		go func(idx int, br BatchRequest) {
			defer wg.Done()
			results[idx] = executeSingle(
				ctx, client, pool, br, bodyLimit,
			)
		}(i, req)
	}

	wg.Wait()
	return results
}

func executeSingle(
	ctx context.Context,
	client *caido.Client,
	pool *SessionPool,
	br BatchRequest,
	bodyLimit int,
) BatchResult {
	result := BatchResult{Label: br.Label}

	// Acquire a session from the pool.
	sessionID, err := pool.Acquire(ctx)
	if err != nil {
		result.Error = fmt.Sprintf("acquire session: %v", err)
		return result
	}
	defer pool.Release(sessionID)

	// Normalize raw request.
	raw := httputil.NormalizeCRLF(br.Raw)

	// Resolve host.
	host := br.Host
	if host == "" {
		host = httputil.ParseHostHeader(br.Raw)
	}
	if host == "" {
		result.Error = "host required (provide in input or Host header)"
		return result
	}

	port := br.Port
	if h, p, splitErr := net.SplitHostPort(host); splitErr == nil {
		host = h
		if port == 0 {
			if pv, convErr := strconv.Atoi(p); convErr == nil {
				port = pv
			}
		}
	}

	useTLS := true
	if br.TLS != nil {
		useTLS = *br.TLS
	}
	if port == 0 {
		if useTLS {
			port = 443
		} else {
			port = 80
		}
	}

	// Snapshot previous entry to detect new one.
	var prevEntryID string
	sessResp, err := client.Replay.GetSession(ctx, sessionID)
	if err == nil && sessResp.ReplaySession != nil &&
		sessResp.ReplaySession.ActiveEntry != nil {
		prevEntryID = sessResp.ReplaySession.ActiveEntry.Id
	}

	rawB64 := base64.StdEncoding.EncodeToString([]byte(raw))
	taskInput := &gen.StartReplayTaskInput{
		Connection: gen.ConnectionInfoInput{
			Host:  host,
			Port:  port,
			IsTLS: useTLS,
		},
		Raw: rawB64,
		Settings: gen.ReplayEntrySettingsInput{
			Placeholders:        []gen.ReplayPlaceholderInput{},
			UpdateContentLength: true,
			ConnectionClose:     false,
		},
	}

	// Send request.
	_, err = client.Replay.SendRequest(ctx, sessionID, taskInput)
	if err != nil {
		result.Error = fmt.Sprintf("send: %v", err)
		return result
	}

	// Poll for response with a per-request timeout.
	pollCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	entry, err := PollForEntry(pollCtx, client, sessionID, prevEntryID)
	if err != nil {
		result.Error = fmt.Sprintf("poll: %v", err)
		return result
	}

	if entry.Request != nil {
		result.Request = httputil.ParseBase64(
			entry.Request.Raw, true, false, 0, 0,
		)
		if entry.Request.Response != nil {
			resp := entry.Request.Response
			result.StatusCode = resp.StatusCode
			result.RoundtripMs = resp.RoundtripTime
			result.Response = httputil.ParseBase64(
				resp.Raw, true, true, 0, bodyLimit,
			)
		}
	}

	return result
}
