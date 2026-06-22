package replay

import (
	"context"
	"encoding/base64"
	"fmt"
	"sync"
	"time"

	caido "github.com/caido-community/sdk-go"
	gen "github.com/caido-community/sdk-go/graphql"
)

const (
	pollInitInterval = 50 * time.Millisecond
	pollMaxInterval  = 500 * time.Millisecond
	PollMaxRetries   = 20
)

var (
	defaultSessionID string
	sessionMu        sync.Mutex
)

// GetOrCreateSession returns the user-provided session ID, the cached
// default session, or creates a new empty session and caches it.
//
// Caido 0.57 sessions created without a request source have no entry, so
// callers that need to send must seed the session via Send/SendOnSession.
func GetOrCreateSession(
	ctx context.Context, client *caido.Client, inputID string,
) (string, error) {
	if inputID != "" {
		return inputID, nil
	}
	sessionMu.Lock()
	defer sessionMu.Unlock()
	if defaultSessionID != "" {
		return defaultSessionID, nil
	}
	// Caido 0.57 GraphQL contract requires kind on CreateReplaySession.
	// Pass an explicit HTTP default instead of nil so we don't rely on
	// sdk-go's empty-string fallback (fragile across SDK pins).
	sessionID, _, err := client.Replay.CreateSession(
		ctx, &gen.CreateReplaySessionInput{Kind: gen.ReplaySessionKindHttp},
	)
	if err != nil {
		return "", fmt.Errorf("create replay session: %w", err)
	}
	defaultSessionID = sessionID
	return defaultSessionID, nil
}

func ResetDefaultSession(newID string) {
	sessionMu.Lock()
	defaultSessionID = newID
	sessionMu.Unlock()
}

// SendResult holds the outcome of a Send: the session used, the previous
// active entry ID (for poll filtering), and any task-in-progress signal.
type SendResult struct {
	SessionID       string
	PreviousEntryID string
}

// Send dispatches a raw HTTP request on a replay session using the Caido
// 0.57 flow: it updates the active entry's draft (or seeds a new session
// with the request when the session has no entry) and starts the task.
//
// On a busy session (TaskInProgressUserError) it transparently creates a
// fresh seeded session and retries once. When the default session was
// used and replaced, the cache is updated. Returns the session ID used
// and the previous active entry ID so callers can poll for the new entry.
func Send(
	ctx context.Context,
	client *caido.Client,
	sessionID, rawRequest string,
	conn caido.ReplayConnection,
	cacheReplacement bool,
) (*SendResult, error) {
	rawB64 := base64.StdEncoding.EncodeToString([]byte(rawRequest))

	prevEntryID, err := sendOnSession(ctx, client, sessionID, rawB64, conn)
	if err == nil {
		return &SendResult{SessionID: sessionID, PreviousEntryID: prevEntryID}, nil
	}

	// Session busy or send failed: create a fresh seeded session and retry.
	newID, _, createErr := client.Replay.CreateSessionWithRaw(ctx, conn, rawB64)
	if createErr != nil {
		return nil, fmt.Errorf(
			"send failed (%v) and fallback session create failed: %w",
			err, createErr,
		)
	}
	if cacheReplacement {
		ResetDefaultSession(newID)
	}
	// The fresh session is seeded with the request; just start the task.
	if _, startErr := client.Replay.StartTask(ctx, newID); startErr != nil {
		return nil, fmt.Errorf("start task on fallback session: %w", startErr)
	}
	return &SendResult{SessionID: newID, PreviousEntryID: ""}, nil
}

// sendOnSession updates the draft of the session's active entry (or seeds
// the session if it has none) and starts the task. Returns the previous
// active entry ID. Returns an error when the session is busy.
func sendOnSession(
	ctx context.Context,
	client *caido.Client,
	sessionID, rawB64 string,
	conn caido.ReplayConnection,
) (string, error) {
	sess, err := client.Replay.GetSession(ctx, sessionID)
	if err != nil {
		return "", fmt.Errorf("get session: %w", err)
	}
	if sess == nil {
		return "", fmt.Errorf("session %s not found", sessionID)
	}

	prevEntryID := sess.ActiveEntryID

	if sess.ActiveEntryID == "" {
		// Empty session (0.57 creates these with no entry). We cannot
		// update a draft; the caller's session is unusable for sending.
		// Signal failure so Send creates a fresh seeded session.
		return "", fmt.Errorf("session %s has no entry to send", sessionID)
	}

	if err := client.Replay.UpdateEntryDraft(
		ctx, sess.ActiveEntryID, conn, rawB64, nil,
	); err != nil {
		return "", fmt.Errorf("update draft: %w", err)
	}

	startResp, err := client.Replay.StartTask(ctx, sessionID)
	if err != nil {
		return "", fmt.Errorf("start task: %w", err)
	}
	if caido.IsTaskInProgress(startResp) {
		return "", fmt.Errorf("task in progress on session %s", sessionID)
	}
	return prevEntryID, nil
}

// PollForEntry polls the session until a new active entry (different from
// prevEntryID) has a response, then returns that entry in domain form.
func PollForEntry(
	ctx context.Context,
	client *caido.Client,
	sessionID, prevEntryID string,
) (*caido.ReplayEntry, error) {
	interval := pollInitInterval
	for range PollMaxRetries {
		sess, err := client.Replay.GetSession(ctx, sessionID)
		if err != nil {
			return nil, fmt.Errorf("poll session: %w", err)
		}
		if sess != nil && sess.ActiveEntryID != "" &&
			sess.ActiveEntryID != prevEntryID {
			entry, err := client.Replay.GetEntry(ctx, sess.ActiveEntryID, "")
			if err != nil {
				return nil, fmt.Errorf("poll entry: %w", err)
			}
			if entry != nil && entry.Request != nil &&
				entry.Request.Response != nil {
				return entry, nil
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
		interval = min(interval*2, pollMaxInterval)
	}
	return nil, fmt.Errorf("timed out waiting for response")
}
