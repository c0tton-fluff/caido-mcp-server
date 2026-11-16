package scope

import "testing"

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		name string
		host string
		glob string
		want bool
	}{
		{"exact literal match", "example.com", "example.com", true},
		{"case insensitive literal", "Example.COM", "example.com", true},
		{"wildcard subdomain matches", "api.example.com", "*.example.com", true},
		{"wildcard subdomain case insensitive", "API.EXAMPLE.COM", "*.example.com", true},
		{"wildcard subdomain multi-level", "a.b.example.com", "*.example.com", true},
		{"bare domain does not match subdomain wildcard", "example.com", "*.example.com", false},
		{"lookalike suffix rejected", "evil-example.com", "*.example.com", false},
		{"unrelated host rejected", "other.com", "*.example.com", false},
		{"bare star matches anything", "anything.test", "*", true},
		{"bare star matches empty-ish host", "x", "*", true},
		{"partial substring is not a match", "example.com.evil.com", "example.com", false},
		{"empty glob never matches", "example.com", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchGlob(tt.host, tt.glob)
			if got != tt.want {
				t.Errorf("MatchGlob(%q, %q) = %v, want %v", tt.host, tt.glob, got, tt.want)
			}
		})
	}
}

func TestParseHost(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"bare host", "example.com", "example.com"},
		{"bare host with port", "example.com:8080", "example.com"},
		{"http url", "http://example.com", "example.com"},
		{"https url with port and path", "https://example.com:8443/path", "example.com"},
		{"mixed case normalized to lower", "HTTPS://Example.COM/Path", "example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseHost(tt.raw)
			if got != tt.want {
				t.Errorf("ParseHost(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

// TestDecide covers the decision enforcement acts on: any scope that allows
// and does not deny puts the host in scope; a denylist match in one scope
// does not veto another scope that allows it; no scopes means nothing is in
// scope; and an empty allowlist follows Caido in matching everything.
func TestDecide(t *testing.T) {
	prod := Scope{ID: "1", Name: "prod", Allowlist: []string{"*.example.com", "example.com"}, Denylist: []string{"admin.example.com"}}
	lab := Scope{ID: "2", Name: "lab", Allowlist: []string{"admin.example.com", "10.0.0.5"}}
	open := Scope{ID: "3", Name: "open"}
	tests := []struct {
		name      string
		scopes    []Scope
		host      string
		want      bool
		wantScope string
		wantRule  string
	}{
		{"allowed by wildcard", []Scope{prod}, "api.example.com", true, "1", "*.example.com"},
		{"allowed bare domain", []Scope{prod}, "example.com", true, "1", "example.com"},
		{"denied by denylist", []Scope{prod}, "admin.example.com", false, "1", "admin.example.com"},
		{"deny in one scope, allow in another", []Scope{prod, lab}, "admin.example.com", true, "2", "admin.example.com"},
		{"lookalike host refused", []Scope{prod}, "example.com.evil.test", false, "", ""},
		{"suffix lookalike refused", []Scope{prod}, "evilexample.com", false, "", ""},
		{"ip literal must be listed", []Scope{prod, lab}, "10.0.0.5", true, "2", "10.0.0.5"},
		{"unlisted ip refused", []Scope{prod, lab}, "10.0.0.6", false, "", ""},
		{"no scopes means nothing in scope", nil, "example.com", false, "", ""},
		{"empty allowlist matches everything", []Scope{open}, "anything.test", true, "3", "* (empty allowlist)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := Decide(tt.scopes, tt.host)
			if v.InScope != tt.want || v.ScopeID != tt.wantScope || v.Rule != tt.wantRule {
				t.Fatalf("Decide = %+v, want inScope=%v scope=%q rule=%q", v, tt.want, tt.wantScope, tt.wantRule)
			}
			if v.Reason == "" || v.Host != tt.host {
				t.Fatalf("verdict must carry the host and a reason: %+v", v)
			}
		})
	}
}
