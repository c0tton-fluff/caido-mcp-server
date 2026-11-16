// Package scope decides whether a host is inside a Caido project's scope.
// Caido exposes no scope-check operation, so this is a local evaluation of
// the same allowlist and denylist host globs, shared by caido_is_in_scope
// (which reports the decision) and scope enforcement (which acts on it).
package scope

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
)

// Scope is one project scope.
type Scope struct {
	ID, Name            string
	Allowlist, Denylist []string
}

// Verdict is the decision for one host. ScopeID, ScopeName and Rule name
// the scope and entry that decided it: the allowing scope when InScope, or
// the scope whose denylist excluded the host; all empty when no allowlist
// matched.
type Verdict struct {
	Host               string
	InScope            bool
	ScopeID, ScopeName string
	Rule               string
	Reason             string
}

// ParseHost extracts a bare, lowercased, port-stripped host from either a
// bare host string ("example.com", "example.com:8080") or a full URL
// ("https://example.com:8443/path"). If the input parses as a URL with a
// host component, that host is used; otherwise the whole input is treated
// as the host, with any trailing port stripped.
func ParseHost(raw string) string {
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

// MatchGlob reports whether host fully matches a Caido scope glob
// pattern. Caido allow/deny entries are host globs where '*' matches any
// run of characters (including none); everything else must match
// literally. The match is case-insensitive and anchored to the entire
// string, not a substring.
func MatchGlob(host, glob string) bool {
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
		if MatchGlob(host, glob) {
			return glob, true
		}
	}
	return "", false
}

// matchDenylist returns the first denylist entry that host matches.
func matchDenylist(host string, denylist []string) (rule string, ok bool) {
	for _, glob := range denylist {
		if MatchGlob(host, glob) {
			return glob, true
		}
	}
	return "", false
}

// Decide evaluates host against scopes. The first scope whose allowlist
// matches and whose denylist does not puts the host in scope. Otherwise the
// first scope that allowed but then denied it is reported as the reason.
// host must already be a bare host (see ParseHost).
func Decide(scopes []Scope, host string) Verdict {
	v := Verdict{Host: host}
	denied := false
	for _, s := range scopes {
		allowRule, allowed := matchAllowlist(host, s.Allowlist)
		if !allowed {
			continue
		}
		denyRule, isDenied := matchDenylist(host, s.Denylist)
		if !isDenied {
			return Verdict{
				Host: host, InScope: true, ScopeID: s.ID, ScopeName: s.Name, Rule: allowRule,
				Reason: fmt.Sprintf(
					"host %q matches allowlist rule %q in scope %q and is not denied",
					host, allowRule, s.Name,
				),
			}
		}
		if !denied {
			denied = true
			v.ScopeID, v.ScopeName, v.Rule = s.ID, s.Name, denyRule
			v.Reason = fmt.Sprintf(
				"host %q matches the allowlist of scope %q but is excluded by denylist rule %q",
				host, s.Name, denyRule,
			)
		}
	}
	if !denied {
		v.Reason = fmt.Sprintf("host %q does not match any scope's allowlist", host)
	}
	return v
}
