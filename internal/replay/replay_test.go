package replay

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/c0tton-fluff/caido-mcp-server/internal/testutil"
	caido "github.com/caido-community/sdk-go"
)

// recordingHandler wraps a testutil.MockHandler, capturing the GraphQL
// operationName and variables of every request so tests can assert which
// operations the SDK actually issued (the base mock only queues responses,
// it does not record calls).
type recordingHandler struct {
	inner *testutil.MockHandler
	mu    sync.Mutex
	ops   []recordedOp
}

type recordedOp struct {
	Name string
	Vars json.RawMessage
}

func (h *recordingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}
	var req struct {
		OperationName string          `json:"operationName"`
		Variables     json.RawMessage `json:"variables"`
	}
	_ = json.Unmarshal(body, &req)
	h.mu.Lock()
	h.ops = append(h.ops, recordedOp{Name: req.OperationName, Vars: req.Variables})
	h.mu.Unlock()
	// Replay the consumed body to the underlying mock handler.
	r.Body = io.NopCloser(bytes.NewReader(body))
	h.inner.ServeHTTP(w, r)
}

// find returns the first recorded operation with the given name.
func (h *recordingHandler) find(name string) (recordedOp, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, op := range h.ops {
		if op.Name == name {
			return op, true
		}
	}
	return recordedOp{}, false
}

// newRecordingEnv builds a caido.Client whose transport records every
// GraphQL operation and delegates responses to a testutil.MockHandler.
func newRecordingEnv(t *testing.T) (*caido.Client, *testutil.MockHandler, *recordingHandler) {
	t.Helper()
	mock := testutil.NewMockHandler()
	rec := &recordingHandler{inner: mock}
	server := httptest.NewServer(rec)
	t.Cleanup(server.Close)

	client, err := caido.NewClient(caido.Options{
		URL:  server.URL,
		Auth: caido.PATAuth("test-token"),
	})
	if err != nil {
		t.Fatalf("failed to create test client: %v", err)
	}
	return client, mock, rec
}

// deletedIDs decodes the ids variable of a recorded DeleteReplaySessions op.
func deletedIDs(t *testing.T, op recordedOp) []string {
	t.Helper()
	var vars struct {
		IDs []string `json:"ids"`
	}
	if err := json.Unmarshal(op.Vars, &vars); err != nil {
		t.Fatalf("decode delete vars %q: %v", string(op.Vars), err)
	}
	return vars.IDs
}

// TestSend_FallbackDeletesOrphanedDefaultSession verifies that when the
// cached default (empty) session cannot send and Send falls back to a fresh
// seeded session, the orphaned default session is deleted so it does not
// leak in Caido.
func TestSend_FallbackDeletesOrphanedDefaultSession(t *testing.T) {
	client, mock, rec := newRecordingEnv(t)
	t.Cleanup(func() { ResetDefaultSession("") })

	// sendOnSession: the default session has no active entry -> fallback.
	mock.On("GetReplaySession", testutil.GetReplaySessionResponse("default-empty", ""))
	mock.On("CreateReplaySession", testutil.CreateReplaySessionSeededResponse("fallback-new", "entry-9"))
	mock.On("DeleteReplaySessions", map[string]any{
		"deleteReplaySessions": map[string]any{"deletedIds": []string{"default-empty"}},
	})
	mock.On("StartReplayTask", testutil.StartReplayTaskResponse())

	conn := caido.ReplayConnection{Host: "example.com", Port: 80}
	res, err := Send(
		context.Background(), client, "default-empty",
		"GET / HTTP/1.1\r\nHost: example.com\r\n\r\n", conn, true,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.SessionID != "fallback-new" {
		t.Fatalf("want fallback session %q, got %q", "fallback-new", res.SessionID)
	}

	op, ok := rec.find("DeleteReplaySessions")
	if !ok {
		t.Fatal("expected DeleteReplaySessions to be called for the orphaned default session")
	}
	ids := deletedIDs(t, op)
	if len(ids) != 1 || ids[0] != "default-empty" {
		t.Fatalf("want delete ids [default-empty], got %v", ids)
	}
}

// TestSend_FallbackKeepsUserSuppliedSession verifies that a user-supplied
// session (cacheReplacement=false) is never deleted on the fallback path.
func TestSend_FallbackKeepsUserSuppliedSession(t *testing.T) {
	client, mock, rec := newRecordingEnv(t)

	mock.On("GetReplaySession", testutil.GetReplaySessionResponse("user-session", ""))
	mock.On("CreateReplaySession", testutil.CreateReplaySessionSeededResponse("fallback-new", "entry-9"))
	mock.On("StartReplayTask", testutil.StartReplayTaskResponse())

	conn := caido.ReplayConnection{Host: "example.com", Port: 80}
	res, err := Send(
		context.Background(), client, "user-session",
		"GET / HTTP/1.1\r\nHost: example.com\r\n\r\n", conn, false,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.SessionID != "fallback-new" {
		t.Fatalf("want fallback session %q, got %q", "fallback-new", res.SessionID)
	}
	if _, ok := rec.find("DeleteReplaySessions"); ok {
		t.Fatal("must not delete a user-supplied session on fallback")
	}
}

func TestGetOrCreateSession_ReturnsInputID(t *testing.T) {
	ctx := context.Background()
	id, err := GetOrCreateSession(ctx, nil, "user-provided-id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "user-provided-id" {
		t.Fatalf("expected %q, got %q", "user-provided-id", id)
	}
}

func TestResetDefaultSession_UpdatesCache(t *testing.T) {
	ResetDefaultSession("abc")
	t.Cleanup(func() { ResetDefaultSession("") })

	sessionMu.Lock()
	got := defaultSessionID
	sessionMu.Unlock()

	if got != "abc" {
		t.Fatalf("expected cached session %q, got %q", "abc", got)
	}
}

func TestGetOrCreateSession_ReturnsCachedSession(t *testing.T) {
	ResetDefaultSession("abc")
	t.Cleanup(func() { ResetDefaultSession("") })

	ctx := context.Background()
	id, err := GetOrCreateSession(ctx, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "abc" {
		t.Fatalf("expected %q, got %q", "abc", id)
	}
}

func TestConstants(t *testing.T) {
	if pollInitInterval != 50*time.Millisecond {
		t.Fatalf(
			"expected pollInitInterval 50ms, got %v",
			pollInitInterval,
		)
	}
	if pollMaxInterval != 500*time.Millisecond {
		t.Fatalf(
			"expected pollMaxInterval 500ms, got %v",
			pollMaxInterval,
		)
	}
	if PollMaxRetries != 20 {
		t.Fatalf(
			"expected PollMaxRetries 20, got %d", PollMaxRetries,
		)
	}
}
