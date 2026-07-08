package tools

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"

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

// parseHost extracts a bare, lowercased, port-stripped host from either a
// bare host string ("example.com", "example.com:8080") or a full URL
// ("https://example.com:8443/path"). If the input parses as a URL with a
// host component, that host is used; otherwise the whole input is treated
// as the host, with any trailing port stripped.
func parseHost(raw string) string {
	raw = strings.TrimSpace(raw)
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return strings.ToLower(u.Hostname())
	}

	host := raw
	if h, _, err := net.SplitHostPort(raw); err == nil {
		host = h
	}
	return strings.ToLower(host)
}

// hostMatchesGlob reports whether host fully matches a Caido scope glob
// pattern. Caido allow/deny entries are host globs where '*' matches any
// run of characters (including none); everything else must match
// literally. The match is case-insensitive and anchored to the entire
// string, not a substring.
func hostMatchesGlob(host, glob string) bool {
	if glob == "" {
		return false
	}

	segments := strings.Split(glob, "*")
	quoted := make([]string, len(segments))
	for i, seg := range segments {
		quoted[i] = regexp.QuoteMeta(seg)
	}
	pattern := "(?i)^" + strings.Join(quoted, ".*") + "$"

	re, err := regexp.Compile(pattern)
	if err != nil {
		return false
	}
	return re.MatchString(host)
}

// matchAllowlist returns the first allowlist entry that host matches. An
// empty allowlist means "matches everything" per Caido semantics, reported
// back as the synthetic rule "*" (empty allowlist). ok is false if no entry
// (including the empty-allowlist fallback) matches.
func matchAllowlist(host string, allowlist []string) (rule string, ok bool) {
	if len(allowlist) == 0 {
		return "* (empty allowlist)", true
	}
	for _, glob := range allowlist {
		if hostMatchesGlob(host, glob) {
			return glob, true
		}
	}
	return "", false
}

// matchDenylist returns the first denylist entry that host matches.
func matchDenylist(host string, denylist []string) (rule string, ok bool) {
	for _, glob := range denylist {
		if hostMatchesGlob(host, glob) {
			return glob, true
		}
	}
	return "", false
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

		host := parseHost(input.Target)
		if host == "" {
			return nil, IsInScopeOutput{}, fmt.Errorf(
				"could not extract a host from target %q", input.Target,
			)
		}

		resp, err := client.Scopes.List(ctx)
		if err != nil {
			return nil, IsInScopeOutput{}, err
		}

		// First scope where the host is actually in scope (allow match and
		// no deny match) wins outright.
		// If none, remember the first scope that matched the allowlist but
		// was excluded by the denylist, to report as the reason.
		var deniedScope *ScopeRef
		var deniedRule string

		for _, s := range resp.Scopes {
			allowRule, allowed := matchAllowlist(host, s.Allowlist)
			if !allowed {
				continue
			}

			denyRule, denied := matchDenylist(host, s.Denylist)
			if !denied {
				return nil, IsInScopeOutput{
					Host:    host,
					InScope: true,
					MatchedScope: &ScopeRef{
						ID:   s.Id,
						Name: s.Name,
					},
					MatchedRule: allowRule,
					Reason: fmt.Sprintf(
						"host %q matches allowlist rule %q in scope %q and is not denied",
						host, allowRule, s.Name,
					),
				}, nil
			}

			if deniedScope == nil {
				deniedScope = &ScopeRef{ID: s.Id, Name: s.Name}
				deniedRule = denyRule
			}
		}

		if deniedScope != nil {
			return nil, IsInScopeOutput{
				Host:         host,
				InScope:      false,
				MatchedScope: deniedScope,
				MatchedRule:  deniedRule,
				Reason: fmt.Sprintf(
					"host %q matches the allowlist of scope %q but is excluded by denylist rule %q",
					host, deniedScope.Name, deniedRule,
				),
			}, nil
		}

		return nil, IsInScopeOutput{
			Host:    host,
			InScope: false,
			Reason:  fmt.Sprintf("host %q does not match any scope's allowlist", host),
		}, nil
	}
}

// RegisterIsInScopeTool registers the tool with the MCP server
func RegisterIsInScopeTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "caido_is_in_scope",
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
