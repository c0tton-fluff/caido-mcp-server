package replay

import (
	"context"
	"testing"
)

func TestSessionPool_TrackAndSize(t *testing.T) {
	pool, err := NewSessionPool(context.Background(), nil, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pool.Track("a")
	pool.Track("") // empty ids are ignored
	pool.Track("b")
	if got := pool.Size(); got != 2 {
		t.Fatalf("want size 2, got %d", got)
	}
}

// TestSessionPool_CleanupDeletesTrackedSessions verifies Cleanup issues a
// single DeleteReplaySessions call carrying every tracked session id.
func TestSessionPool_CleanupDeletesTrackedSessions(t *testing.T) {
	client, mock, rec := newRecordingEnv(t)
	mock.On("DeleteReplaySessions", map[string]any{
		"deleteReplaySessions": map[string]any{"deletedIds": []string{"s1", "s2"}},
	})

	pool, err := NewSessionPool(context.Background(), client, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pool.Track("s1")
	pool.Track("s2")

	pool.Cleanup(context.Background())

	op, ok := rec.find("DeleteReplaySessions")
	if !ok {
		t.Fatal("expected Cleanup to call DeleteReplaySessions")
	}
	ids := deletedIDs(t, op)
	if len(ids) != 2 || ids[0] != "s1" || ids[1] != "s2" {
		t.Fatalf("want delete ids [s1 s2], got %v", ids)
	}
}

// TestSessionPool_CleanupNoTrackedSessions verifies Cleanup makes no delete
// call when nothing was tracked.
func TestSessionPool_CleanupNoTrackedSessions(t *testing.T) {
	client, _, rec := newRecordingEnv(t)
	pool, err := NewSessionPool(context.Background(), client, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pool.Cleanup(context.Background())
	if _, ok := rec.find("DeleteReplaySessions"); ok {
		t.Fatal("Cleanup with no tracked sessions must not call DeleteReplaySessions")
	}
}

// TestSessionPool_CleanupOnDeleteErrorDoesNotPanic verifies the best-effort
// error path (no mock registered -> the SDK returns a GraphQL error) is
// logged and swallowed rather than crashing Cleanup.
func TestSessionPool_CleanupOnDeleteErrorDoesNotPanic(t *testing.T) {
	// No DeleteReplaySessions response is registered, so the mock returns a
	// GraphQL error which the SDK surfaces. Cleanup must not panic.
	client, _, _ := newRecordingEnv(t)
	pool, err := NewSessionPool(context.Background(), client, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pool.Track("s1")
	pool.Cleanup(context.Background())
}
