package replay

import (
	"errors"
	"strings"
	"testing"
)

func TestExplainUnsupportedKind(t *testing.T) {
	if ExplainUnsupportedKind(nil, "replay session", "1") != nil {
		t.Fatal("nil must stay nil")
	}
	other := errors.New("boom")
	if got := ExplainUnsupportedKind(other, "replay session", "1"); got != other {
		t.Fatalf("unrelated error must pass through unchanged, got %v", got)
	}
	sdk := errors.New(`unexpected concrete type for GetReplaySessionReplaySession: "ReplaySessionHttpOnePipeline"`)
	got := ExplainUnsupportedKind(sdk, "replay session", "42")
	if !errors.Is(got, sdk) {
		t.Fatal("explained error must wrap the original")
	}
	if msg := got.Error(); !strings.Contains(msg, "replay session 42") ||
		!strings.Contains(msg, "caido_list_replay_sessions") {
		t.Fatalf("unhelpful message: %s", msg)
	}
}
