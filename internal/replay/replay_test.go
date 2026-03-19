package replay

import (
	"context"
	"testing"
	"time"
)

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
	if PollInterval != 500*time.Millisecond {
		t.Fatalf(
			"expected PollInterval 500ms, got %v", PollInterval,
		)
	}
	if PollMaxRetries != 20 {
		t.Fatalf(
			"expected PollMaxRetries 20, got %d", PollMaxRetries,
		)
	}
}
