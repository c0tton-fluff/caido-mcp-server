package tools

import (
	"context"
	"fmt"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/scope"
	caido "github.com/caido-community/sdk-go"
)

// Scope enforcement (Options.EnforceScope): every tool that contacts a
// target refuses a host outside the project's scopes, and the tools that
// could move that boundary are withheld.
//
// What it covers: the connection host of caido_send_request,
// caido_edit_request, caido_batch_send, caido_race_pipeline,
// caido_race_window_send and caido_ws_connect, checked where the request
// is handed to Caido (or to the raw socket), so no input shape goes around
// it. What it does not cover: traffic the server does not address itself,
// i.e. forwarding an intercepted browser request, starting an Automate
// session a human configured, or running a workflow.

// scopeLockedTools are withheld while scope is enforced: each one can
// change which hosts count as in scope (editing scopes, or switching to a
// project with different ones). An agent that could call them could let
// itself out.
var scopeLockedTools = []string{
	"caido_create_scope",
	"caido_update_scope",
	"caido_delete_scope",
	"caido_select_project",
	"caido_create_project",
	"caido_delete_project",
}

// enforceScope is the replay.TargetGuard for scope enforcement. The scopes
// are read on every call so a change the operator makes in Caido applies
// to the next request.
//
// A scope with an empty allowlist is ignored here. caido_is_in_scope
// reports such a scope as matching everything, following Caido, but
// enforcement only lets a host through on an explicit allowlist entry:
// "no entries" must not turn the boundary off.
func enforceScope(ctx context.Context, client *caido.Client, host string) error {
	all, err := projectScopes(ctx, client)
	if err != nil {
		return fmt.Errorf("scope enforcement: cannot read scopes, refusing to send: %w", err)
	}
	explicit := make([]scope.Scope, 0, len(all))
	for _, s := range all {
		if len(s.Allowlist) > 0 {
			explicit = append(explicit, s)
		}
	}
	bare := scope.ParseHost(host)
	if bare == "" {
		return fmt.Errorf("scope enforcement: no target host, refusing to send")
	}
	if len(explicit) == 0 {
		return fmt.Errorf(
			"scope enforcement: host %q refused, the project has no scope with an allowlist; "+
				"the operator must define one in Caido", bare,
		)
	}
	if v := scope.Decide(explicit, bare); !v.InScope {
		return fmt.Errorf(
			"scope enforcement: %s; out-of-scope targets are refused (check with caido_is_in_scope)",
			v.Reason,
		)
	}
	return nil
}
