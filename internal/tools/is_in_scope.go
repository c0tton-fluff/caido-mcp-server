package tools

import (
	"context"
	"fmt"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/scope"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// IsInScopeInput is the input for the is_in_scope tool
type IsInScopeInput struct {
	Target string `json:"target" jsonschema:"required,Host or URL to check (e.g. example.com, https://example.com/path)"`
}

// ScopeRef identifies the scope that decided an is_in_scope match.
type ScopeRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// IsInScopeOutput is the output of the is_in_scope tool
type IsInScopeOutput struct {
	Host         string    `json:"host"`
	InScope      bool      `json:"inScope"`
	MatchedScope *ScopeRef `json:"matchedScope,omitempty"`
	MatchedRule  string    `json:"matchedRule,omitempty"`
	Reason       string    `json:"reason"`
}

// isInScopeHandler creates the handler function.
//
// NOTE: sdk-go exposes no native scope-check/test query (verified against
// the generated GraphQL schema and ScopeSDK -- only List/Get/Create/Rename/
// Delete exist). This handler is a local approximation of Caido's scope
// matching: it fetches all scopes via client.Scopes.List and evaluates the
// allow/deny host globs itself.
func isInScopeHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, IsInScopeInput) (*mcp.CallToolResult, IsInScopeOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input IsInScopeInput,
	) (*mcp.CallToolResult, IsInScopeOutput, error) {
		if input.Target == "" {
			return nil, IsInScopeOutput{}, fmt.Errorf("target is required")
		}

		host := scope.ParseHost(input.Target)
		if host == "" {
			return nil, IsInScopeOutput{}, fmt.Errorf(
				"could not extract a host from target %q", input.Target,
			)
		}

		scopes, err := projectScopes(ctx, client)
		if err != nil {
			return nil, IsInScopeOutput{}, err
		}
		v := scope.Decide(scopes, host)
		out := IsInScopeOutput{
			Host: host, InScope: v.InScope, MatchedRule: v.Rule, Reason: v.Reason,
		}
		if v.ScopeID != "" {
			out.MatchedScope = &ScopeRef{ID: v.ScopeID, Name: v.ScopeName}
		}
		return nil, out, nil
	}
}

// projectScopes lists the project's scopes in the form the scope package
// decides on.
func projectScopes(ctx context.Context, client *caido.Client) ([]scope.Scope, error) {
	resp, err := client.Scopes.List(ctx)
	if err != nil {
		return nil, err
	}
	scopes := make([]scope.Scope, 0, len(resp.Scopes))
	for _, s := range resp.Scopes {
		scopes = append(scopes, scope.Scope{
			ID: s.Id, Name: s.Name, Allowlist: s.Allowlist, Denylist: s.Denylist,
		})
	}
	return scopes, nil
}

// RegisterIsInScopeTool registers the tool with the MCP server
func RegisterIsInScopeTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:  "caido_is_in_scope",
		Title: "Check In Scope",
		Description: `Check whether a host or URL is in the project scope. ` +
			`Accepts a bare host ("example.com") or a full URL ` +
			`("https://example.com/path"); the port and path are ignored. ` +
			`Returns whether it is in scope, the matching scope, and the ` +
			`allow/deny rule that decided the result. Local approximation ` +
			`of Caido's scope matching (sdk-go exposes no native scope-check ` +
			`query): fetches scopes and matches host globs.`,
		Annotations: readOnlyAnn(),
	}, isInScopeHandler(client))
}
