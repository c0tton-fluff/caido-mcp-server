package raceattack

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
)

// targetFromURL parses an httptest.Server URL into a non-TLS Target.
func targetFromURL(t *testing.T, raw string) Target {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse server URL %q: %v", raw, err)
	}
	host, portStr, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatalf("split host:port from %q: %v", u.Host, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse port %q: %v", portStr, err)
	}
	return Target{Host: host, Port: port, TLS: false}
}

func TestSendSynchronized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello-race"))
	}))
	defer srv.Close()

	const n = 5
	target := targetFromURL(t, srv.URL)
	reqs := make([]Request, n)
	for i := range reqs {
		reqs[i] = Request{
			Label: fmt.Sprintf("req-%d", i),
			Raw:   "GET / HTTP/1.1\r\nHost: " + target.Host + "\r\nConnection: close\r\n\r\n",
		}
	}

	results := Send(context.Background(), target, reqs, 0)
	if len(results) != n {
		t.Fatalf("want %d results, got %d", n, len(results))
	}
	for i, r := range results {
		if r.Error != "" {
			t.Errorf("result[%d] %s: unexpected error %q", i, r.Label, r.Error)
		}
		if r.StatusCode != http.StatusOK {
			t.Errorf("result[%d] %s: want status 200, got %d (line %q)",
				i, r.Label, r.StatusCode, r.StatusLine)
		}
		if r.Body == "" {
			t.Errorf("result[%d] %s: body is empty", i, r.Label)
		}
	}
}

func TestSendKeepAliveShortBody(t *testing.T) {
	// The server keeps the connection alive (no Connection: close) and
	// returns a small body. readResponse must return promptly once the
	// complete body is read instead of blocking for the full dialTimeout
	// waiting to fill a fixed-size buffer.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ka-body"))
	}))
	defer srv.Close()

	target := targetFromURL(t, srv.URL)
	reqs := []Request{{
		Label: "keepalive",
		// No Connection: close -> Go's server keeps the socket open.
		Raw: "GET / HTTP/1.1\r\nHost: " + target.Host + "\r\n\r\n",
	}}

	results := Send(context.Background(), target, reqs, 0)
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	r := results[0]
	if r.Error != "" {
		t.Fatalf("unexpected error: %q", r.Error)
	}
	if r.StatusCode != http.StatusOK {
		t.Fatalf("want status 200, got %d (line %q)", r.StatusCode, r.StatusLine)
	}
	if r.Body != "ka-body" {
		t.Fatalf("want body %q, got %q", "ka-body", r.Body)
	}
	if r.DurationMs >= 2000 {
		t.Fatalf("keep-alive read blocked too long: %d ms (want < 2000)", r.DurationMs)
	}
}

func TestSendDialFailure(t *testing.T) {
	// Port 1 on loopback should refuse / fail to connect.
	target := Target{Host: "127.0.0.1", Port: 1, TLS: false}
	reqs := []Request{{Label: "dead", Raw: "GET / HTTP/1.1\r\nHost: x\r\n\r\n"}}

	results := Send(context.Background(), target, reqs, 0)
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	if results[0].Error == "" {
		t.Errorf("expected dial error to be populated, got empty")
	}
	if results[0].StatusCode != 0 {
		t.Errorf("want status 0 on dial failure, got %d", results[0].StatusCode)
	}
}

func TestSendValidation(t *testing.T) {
	tests := []struct {
		name    string
		target  Target
		reqs    []Request
		wantErr string
	}{
		{
			name:    "empty host",
			target:  Target{Host: "", Port: 443, TLS: true},
			reqs:    []Request{{Label: "a", Raw: "GET / HTTP/1.1\r\n\r\n"}},
			wantErr: "host",
		},
		{
			name:    "zero requests",
			target:  Target{Host: "example.com", Port: 443, TLS: true},
			reqs:    nil,
			wantErr: "request",
		},
		{
			name:    "port out of range",
			target:  Target{Host: "example.com", Port: 0, TLS: true},
			reqs:    []Request{{Label: "a", Raw: "GET / HTTP/1.1\r\n\r\n"}},
			wantErr: "port",
		},
		{
			name:   "too many requests",
			target: Target{Host: "example.com", Port: 443, TLS: true},
			reqs: func() []Request {
				rs := make([]Request, maxRequests+1)
				for i := range rs {
					rs[i] = Request{Label: "x", Raw: "GET / HTTP/1.1\r\n\r\n"}
				}
				return rs
			}(),
			wantErr: "max",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := Send(context.Background(), tt.target, tt.reqs, 0)
			if len(results) != 1 {
				t.Fatalf("want 1 validation result, got %d", len(results))
			}
			if results[0].Error == "" {
				t.Fatalf("expected validation error containing %q, got none", tt.wantErr)
			}
		})
	}
}

func TestClampBodyLimit(t *testing.T) {
	if got := clampBodyLimit(0); got != defaultBodyLimit {
		t.Errorf("clampBodyLimit(0) = %d, want %d", got, defaultBodyLimit)
	}
	if got := clampBodyLimit(maxBodyLimit + 1); got != maxBodyLimit {
		t.Errorf("clampBodyLimit(over) = %d, want %d", got, maxBodyLimit)
	}
	if got := clampBodyLimit(100); got != 100 {
		t.Errorf("clampBodyLimit(100) = %d, want 100", got)
	}
}

func TestParseStatus(t *testing.T) {
	cases := map[string]int{
		"HTTP/1.1 200 OK":              200,
		"HTTP/1.1 404 Not Found":       404,
		"HTTP/1.1 500":                 500,
		"garbage":                      0,
		"HTTP/1.1 notanumber Whatever": 0,
	}
	for line, want := range cases {
		if got := parseStatus(line); got != want {
			t.Errorf("parseStatus(%q) = %d, want %d", line, got, want)
		}
	}
}
