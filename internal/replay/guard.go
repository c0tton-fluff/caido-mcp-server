package replay

import (
	"context"
	"sync/atomic"

	caido "github.com/caido-community/sdk-go"
)

// TargetGuard decides whether host may be contacted. A non-nil error
// refuses the send and is returned to the caller as is.
type TargetGuard func(ctx context.Context, client *caido.Client, host string) error

// targetGuard is process-wide because every send path in this package (and
// the raw-socket race tool, through CheckTarget) must consult the same
// policy, and the policy is fixed for the life of the server.
var targetGuard atomic.Pointer[TargetGuard]

// SetTargetGuard installs the guard every send consults before anything
// leaves; nil removes it. It is set once at startup (scope enforcement).
func SetTargetGuard(g TargetGuard) {
	if g == nil {
		targetGuard.Store(nil)
		return
	}
	targetGuard.Store(&g)
}

// CheckTarget asks the installed guard whether host may be contacted. With
// no guard installed every host is allowed. Send, SendNew, RunBatch,
// RunPipeline and WSConnect call it themselves; a caller that reaches a
// target by any other route must call it first.
func CheckTarget(ctx context.Context, client *caido.Client, host string) error {
	g := targetGuard.Load()
	if g == nil {
		return nil
	}
	return (*g)(ctx, client, host)
}
