package replay_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/replay"
	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/testutil"
	caido "github.com/caido-community/sdk-go"
)

var errRefused = errors.New("refused by test guard")

// installGuard refuses every host except allowed, records what it was
// asked, and is removed when the test ends.
func installGuard(t *testing.T, allowed string) *[]string {
	t.Helper()
	var asked []string
	replay.SetTargetGuard(func(_ context.Context, _ *caido.Client, host string) error {
		asked = append(asked, host)
		if host == allowed {
			return nil
		}
		return errRefused
	})
	t.Cleanup(func() { replay.SetTargetGuard(nil) })
	return &asked
}

// TestTargetGuardOnEverySendPath calls each entry point that hands a
// request to Caido with a guard that refuses the host. Each must return the
// guard's error and reach Caido with nothing: the mock has no operations
// registered, so any GraphQL call would surface as a different error.
func TestTargetGuardOnEverySendPath(t *testing.T) {
	conn := caido.ReplayConnection{Host: "out.example", Port: 443, IsTLS: true}
	const raw = "GET / HTTP/1.1\r\nHost: out.example\r\n\r\n"
	paths := map[string]func(ctx context.Context, c *caido.Client) error{
		"Send": func(ctx context.Context, c *caido.Client) error {
			_, err := replay.Send(ctx, c, "sess-1", raw, conn, false)
			return err
		},
		"SendNew": func(ctx context.Context, c *caido.Client) error {
			_, err := replay.SendNew(ctx, c, raw, conn, "", "")
			return err
		},
		"RunPipeline": func(ctx context.Context, c *caido.Client) error {
			_, err := replay.RunPipeline(ctx, c,
				[]replay.PipelineRequest{{Raw: raw, Conn: conn}}, replay.PipelineOptions{})
			return err
		},
		"WSConnect": func(ctx context.Context, c *caido.Client) error {
			_, err := replay.WSConnect(ctx, c, replay.WSConnectOptions{Raw: raw, Conn: conn})
			return err
		},
		"WSConnect prepare only": func(ctx context.Context, c *caido.Client) error {
			_, err := replay.WSConnect(ctx, c, replay.WSConnectOptions{Raw: raw, Conn: conn, SkipOpen: true})
			return err
		},
		"RunBatch": func(ctx context.Context, c *caido.Client) error {
			res := replay.RunBatch(ctx, c, []replay.BatchRequest{{Label: "a", Raw: raw}}, 1, 0)
			if len(res) != 1 || !strings.Contains(res[0].Error, errRefused.Error()) {
				return errors.New("batch item was not refused: " + res[0].Error)
			}
			return errRefused
		},
	}
	for name, call := range paths {
		t.Run(name, func(t *testing.T) {
			env := testutil.NewTestEnv(t)
			asked := installGuard(t, "in.example")
			err := call(context.Background(), env.Client)
			if !errors.Is(err, errRefused) {
				t.Fatalf("want the guard's refusal, got %v", err)
			}
			if len(*asked) == 0 || (*asked)[0] != "out.example" {
				t.Fatalf("guard was asked about %v, want out.example", *asked)
			}
		})
	}
}

// TestCheckTargetWithoutGuard: no guard means every host is allowed, and
// removing a guard restores that.
func TestCheckTargetWithoutGuard(t *testing.T) {
	replay.SetTargetGuard(nil)
	if err := replay.CheckTarget(context.Background(), nil, "anything.example"); err != nil {
		t.Fatalf("no guard installed, got %v", err)
	}
	installGuard(t, "in.example")
	if err := replay.CheckTarget(context.Background(), nil, "out.example"); !errors.Is(err, errRefused) {
		t.Fatalf("guard installed, got %v", err)
	}
	if err := replay.CheckTarget(context.Background(), nil, "in.example"); err != nil {
		t.Fatalf("allowed host refused: %v", err)
	}
	replay.SetTargetGuard(nil)
	if err := replay.CheckTarget(context.Background(), nil, "out.example"); err != nil {
		t.Fatalf("guard removed, got %v", err)
	}
}
