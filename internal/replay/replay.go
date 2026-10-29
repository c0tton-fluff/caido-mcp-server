package replay

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
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

// swapDefaultSession replaces the cached default session with newID only
// when it still equals oldID, and reports whether it did. A concurrent Send
// that already replaced the default wins; the loser keeps its own session
// without clobbering the winner's cache entry.
func swapDefaultSession(oldID, newID string) bool {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	if defaultSessionID != oldID {
		return false
	}
	defaultSessionID = newID
	return true
}

// SendResult holds the outcome of a Send: the session used, the previous
// active entry ID (for poll filtering), and any task-in-progress signal.
// When the fallback replaced the caller's session, SessionID differs from
// the one passed to Send and DeletedOriginal reports whether the original
// was deleted (an empty orphan) or kept (it has history).
type SendResult struct {
	SessionID       string
	PreviousEntryID string
	DeletedOriginal bool
}

// Send dispatches a raw HTTP request on a replay session using the Caido
// 0.57 flow: it updates the active entry's draft (or seeds a new session
// with the request when the session has no entry) and starts the task.
//
// On a busy or empty session it transparently creates a fresh seeded
// session and retries once. When the default session was used and
// replaced, the cache is updated. A user-supplied session's name and
// collection carry over to the replacement, and an empty one (as left by
// create_replay_session) is deleted so the request lands where the caller
// put it rather than in an unnamed session in the default collection. A
// user-supplied session that cannot be read is an error, not a fallback.
// Returns the session ID used and the previous active entry ID so callers
// can poll for the new entry.
func Send(
	ctx context.Context,
	client *caido.Client,
	sessionID, rawRequest string,
	conn caido.ReplayConnection,
	cacheReplacement bool,
) (*SendResult, error) {
	rawB64 := base64.StdEncoding.EncodeToString([]byte(rawRequest))

	sess, prevEntryID, err := sendOnSession(ctx, client, sessionID, rawB64, conn)
	if err == nil {
		return &SendResult{SessionID: sessionID, PreviousEntryID: prevEntryID}, nil
	}
	// A user-supplied session that cannot be read has no name or
	// collection to carry over; a fallback would file the request as an
	// unnamed session in the default collection. Report it instead.
	if !cacheReplacement && sess == nil {
		return nil, fmt.Errorf(
			"%w; list_replay_sessions shows current IDs (a send to an "+
				"empty session replaces it and returns the new sessionId)",
			err,
		)
	}

	// Session busy or send failed: create a fresh seeded session and retry.
	var collectionID, name string
	if !cacheReplacement && sess != nil {
		collectionID, name = sess.Collection.ID, sess.Name
	}
	newID, err2 := createSeeded(ctx, client, rawB64, conn, collectionID, name)
	if err2 != nil {
		return nil, fmt.Errorf(
			"send failed (%v) and fallback session create failed: %w",
			err, err2,
		)
	}
	// The fresh session is seeded with the request; start its task before
	// touching the original, so a failed start leaves the original intact.
	if _, startErr := client.Replay.StartTask(ctx, newID); startErr != nil {
		return nil, fmt.Errorf(
			"start task on fallback session %s: %w", newID, startErr,
		)
	}
	// An empty session (the auto-created default, or one left by
	// create_replay_session) is a hollow orphan in Caido 0.57 once
	// replaced; best-effort delete it to avoid leaking a stray session per
	// send. A session with entries (busy), default or user-supplied,
	// keeps its history: a concurrent call may still be polling it.
	deleted := false
	if sess != nil && sess.ActiveEntryID == "" {
		if _, delErr := client.Replay.DeleteSessions(
			ctx, []string{sessionID},
		); delErr != nil {
			fmt.Fprintf(
				os.Stderr,
				"replay: failed to delete orphaned session %s: %v\n",
				sessionID, delErr,
			)
		} else {
			deleted = true
		}
	}
	if cacheReplacement {
		// Compare-and-swap: if another call already replaced the default,
		// keep this call's session (it holds this request) and leave the
		// winner's cache entry alone.
		swapDefaultSession(sessionID, newID)
	}
	return &SendResult{
		SessionID: newID, PreviousEntryID: "", DeletedOriginal: deleted,
	}, nil
}

// SendNew creates a fresh replay session seeded with rawRequest, places it
// in collectionID and names it name (both optional), and starts the task.
// This is the one-shot path for callers that want each request filed as
// its own titled session instead of reusing the default one.
func SendNew(
	ctx context.Context,
	client *caido.Client,
	rawRequest string,
	conn caido.ReplayConnection,
	collectionID, name string,
) (*SendResult, error) {
	rawB64 := base64.StdEncoding.EncodeToString([]byte(rawRequest))
	newID, err := createSeeded(ctx, client, rawB64, conn, collectionID, name)
	if err != nil {
		return nil, err
	}
	if _, err := client.Replay.StartTask(ctx, newID); err != nil {
		return nil, fmt.Errorf("start task on session %s: %w", newID, err)
	}
	return &SendResult{SessionID: newID, PreviousEntryID: ""}, nil
}

// createSeeded creates an HTTP replay session seeded with rawB64, in
// collectionID when set, and renames it to name when set.
func createSeeded(
	ctx context.Context,
	client *caido.Client,
	rawB64 string,
	conn caido.ReplayConnection,
	collectionID, name string,
) (string, error) {
	input := &gen.CreateReplaySessionInput{
		Kind:          gen.ReplaySessionKindHttp,
		RequestSource: caido.NewRawRequestSource(conn, rawB64),
	}
	if collectionID != "" {
		input.CollectionId = &collectionID
	}
	newID, _, err := client.Replay.CreateSession(ctx, input)
	if err != nil {
		return "", fmt.Errorf("create replay session: %w", err)
	}
	if name != "" {
		if _, err := client.Replay.RenameSession(ctx, newID, name); err != nil {
			// Do not leak the half-built session: nothing returns its ID.
			if _, delErr := client.Replay.DeleteSessions(
				ctx, []string{newID},
			); delErr != nil {
				fmt.Fprintf(
					os.Stderr,
					"replay: failed to delete orphaned session %s: %v\n",
					newID, delErr,
				)
			}
			return "", fmt.Errorf("rename session %s: %w", newID, err)
		}
	}
	return newID, nil
}

// sendOnSession updates the draft of the session's active entry and starts
// the task. Returns the fetched session (nil if it could not be read) and
// the previous active entry ID. Returns an error when the session is busy
// or has no entry to send.
func sendOnSession(
	ctx context.Context,
	client *caido.Client,
	sessionID, rawB64 string,
	conn caido.ReplayConnection,
) (*caido.ReplaySession, string, error) {
	sess, err := client.Replay.GetSession(ctx, sessionID)
	if err != nil {
		return nil, "", fmt.Errorf("get session: %w", err)
	}
	if sess == nil {
		return nil, "", fmt.Errorf("session %s not found", sessionID)
	}

	prevEntryID := sess.ActiveEntryID

	if sess.ActiveEntryID == "" {
		// Empty session (0.57 creates these with no entry). We cannot
		// update a draft; the caller's session is unusable for sending.
		// Signal failure so Send creates a fresh seeded session.
		return sess, "", fmt.Errorf("session %s has no entry to send", sessionID)
	}

	if err := client.Replay.UpdateEntryDraft(
		ctx, sess.ActiveEntryID, conn, rawB64, nil,
	); err != nil {
		return sess, "", fmt.Errorf("update draft: %w", err)
	}

	startResp, err := client.Replay.StartTask(ctx, sessionID)
	if err != nil {
		return sess, "", fmt.Errorf("start task: %w", err)
	}
	if caido.IsTaskInProgress(startResp) {
		return sess, "", fmt.Errorf("task in progress on session %s", sessionID)
	}
	return sess, prevEntryID, nil
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
