package httputil

import (
	"strings"
	"testing"
)

func TestRedactRawHeaders(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		allow       string // CAIDO_ALLOW_SENSITIVE_HEADERS value; "" leaves it unset
		wantContain []string
		wantAbsent  []string
	}{
		{
			name: "redacts authorization and cookie in request",
			raw: "GET /admin HTTP/1.1\r\n" +
				"Host: example.com\r\n" +
				"Authorization: Bearer supersecrettoken\r\n" +
				"Cookie: session=abc123def\r\n" +
				"Accept: */*\r\n" +
				"\r\n" +
				"request body here",
			wantContain: []string{
				"GET /admin HTTP/1.1",
				"Host: example.com",
				"Authorization: [REDACTED]",
				"Cookie: [REDACTED]",
				"Accept: */*",
				"request body here",
			},
			wantAbsent: []string{
				"supersecrettoken",
				"session=abc123def",
			},
		},
		{
			name: "redacts set-cookie in response",
			raw: "HTTP/1.1 200 OK\r\n" +
				"Content-Type: text/html\r\n" +
				"Set-Cookie: auth=topsecretvalue; HttpOnly\r\n" +
				"\r\n" +
				"<html>ok</html>",
			wantContain: []string{
				"HTTP/1.1 200 OK",
				"Content-Type: text/html",
				"Set-Cookie: [REDACTED]",
				"<html>ok</html>",
			},
			wantAbsent: []string{"topsecretvalue"},
		},
		{
			name: "leaves non-sensitive headers and body untouched",
			raw: "POST /api HTTP/1.1\r\n" +
				"Host: example.com\r\n" +
				"Content-Type: application/json\r\n" +
				"\r\n" +
				"Authorization: not-a-real-header-in-body",
			wantContain: []string{
				"POST /api HTTP/1.1",
				"Host: example.com",
				"Content-Type: application/json",
				// A header-looking line in the BODY must survive verbatim.
				"Authorization: not-a-real-header-in-body",
			},
			wantAbsent: []string{"[REDACTED]"},
		},
		{
			name: "handles bare LF line endings",
			raw: "GET / HTTP/1.1\n" +
				"Authorization: Bearer lftoken\n" +
				"\n" +
				"body",
			wantContain: []string{
				"GET / HTTP/1.1",
				"Authorization: [REDACTED]",
				"body",
			},
			wantAbsent: []string{"lftoken"},
		},
		{
			name: "redacts case-insensitively",
			raw: "GET / HTTP/1.1\r\n" +
				"AUTHORIZATION: Bearer casetoken\r\n" +
				"\r\n",
			wantContain: []string{"AUTHORIZATION: [REDACTED]"},
			wantAbsent:  []string{"casetoken"},
		},
		{
			name:  "opt-out disables redaction",
			allow: "1",
			raw: "GET / HTTP/1.1\r\n" +
				"Authorization: Bearer keepme\r\n" +
				"Cookie: session=keepmetoo\r\n" +
				"\r\n",
			wantContain: []string{
				"Authorization: Bearer keepme",
				"Cookie: session=keepmetoo",
			},
			wantAbsent: []string{"[REDACTED]"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.allow != "" {
				t.Setenv("CAIDO_ALLOW_SENSITIVE_HEADERS", tt.allow)
			}
			got := RedactRawHeaders(tt.raw)
			for _, want := range tt.wantContain {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q\n---\n%s", want, got)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("output should not contain %q\n---\n%s", absent, got)
				}
			}
		})
	}
}

func TestRedactRawHeaders_Empty(t *testing.T) {
	if got := RedactRawHeaders(""); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}
