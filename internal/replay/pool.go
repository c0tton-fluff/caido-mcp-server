package replay

import (
	"context"
	"fmt"
	"os"
	"sync"

	caido "github.com/caido-community/sdk-go"
)

// SessionPool bounds concurrency for parallel batch sends and tracks the
// replay sessions created during the batch so they can be cleaned up.
//
// Caido 0.57 replay sessions created without a request source have no
// entry, and a session's draft can only be updated on an existing entry.
// Pre-creating empty sessions is therefore useless for sending; instead
// each batch request creates its own session seeded with its request (see
// Send). The pool just caps concurrency and records created session IDs.
type SessionPool struct {
	client *caido.Client
	slots  chan struct{}
	mu     sync.Mutex
	ids    []string
}

// NewSessionPool creates a pool that allows up to n concurrent sends.
func NewSessionPool(
	ctx context.Context, client *caido.Client, n int,
) (*SessionPool, error) {
	if n < 1 {
		n = 1
	}
	if n > 50 {
		n = 50
	}
	pool := &SessionPool{
		client: client,
		slots:  make(chan struct{}, n),
		ids:    make([]string, 0, n),
	}
	return pool, nil
}

// Acquire blocks until a concurrency slot is free.
func (p *SessionPool) Acquire(ctx context.Context) error {
	select {
	case p.slots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release frees a concurrency slot.
func (p *SessionPool) Release() {
	<-p.slots
}

// Track records a created session ID for later cleanup.
func (p *SessionPool) Track(id string) {
	if id == "" {
		return
	}
	p.mu.Lock()
	p.ids = append(p.ids, id)
	p.mu.Unlock()
}

// Size returns the number of sessions tracked for cleanup.
func (p *SessionPool) Size() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.ids)
}

// Cleanup deletes all sessions created during the batch.
func (p *SessionPool) Cleanup(ctx context.Context) {
	p.mu.Lock()
	ids := make([]string, len(p.ids))
	copy(ids, p.ids)
	p.mu.Unlock()

	if len(ids) == 0 {
		return
	}
	// Best-effort cleanup: log a delete failure to stderr so a leaked batch
	// session is observable, but never fail the batch on it.
	if _, err := p.client.Replay.DeleteSessions(ctx, ids); err != nil {
		fmt.Fprintf(
			os.Stderr,
			"replay: failed to delete %d batch session(s): %v\n",
			len(ids), err,
		)
	}
}
