package httputil

import (
	"strings"
	"testing"
)

func TestParseCurl(t *testing.T) {
	tests := []struct {
		name     string
		cmd      string
		wantRaw  string
		wantHost string
		wantPort int
		wantTLS  bool
	}{
		{
			name:     "plain GET defaults to http and port 80",
			cmd:      `curl example.com/a?b=1`,
			wantRaw:  "GET /a?b=1 HTTP/1.1\r\nHost: example.com\r\n\r\n",
			wantHost: "example.com", wantPort: 80,
		},
		{
			name: "chrome copy as cURL (bash) with $'...' body and continuations",
			cmd: "curl 'https://api.test/v1/login' \\\n" +
				"  -H 'accept: application/json' \\\n" +
				"  -H 'content-type: application/json' \\\n" +
				"  -b 'sid=abc; theme=dark' \\\n" +
				"  --data-raw $'{\"user\":\"o\\'neil\",\"note\":\"a\\nb\"}' \\\n" +
				"  --compressed",
			wantRaw: "POST /v1/login HTTP/1.1\r\nHost: api.test\r\n" +
				"accept: application/json\r\ncontent-type: application/json\r\n" +
				"Cookie: sid=abc; theme=dark\r\nContent-Length: 30\r\n\r\n" +
				"{\"user\":\"o'neil\",\"note\":\"a\nb\"}",
			wantHost: "api.test", wantPort: 443, wantTLS: true,
		},
		{
			name: "firefox style: -X before URL, double quotes, explicit port",
			cmd:  `curl -X PUT "https://h.test:8443/x" -H "Authorization: Bearer t\"k" --data-binary "a=1"`,
			wantRaw: "PUT /x HTTP/1.1\r\nHost: h.test:8443\r\nAuthorization: Bearer t\"k\r\n" +
				"Content-Type: application/x-www-form-urlencoded\r\nContent-Length: 3\r\n\r\na=1",
			wantHost: "h.test", wantPort: 8443, wantTLS: true,
		},
		{
			name: "multiple -d join with & and attached -XPATCH",
			cmd:  `curl -XPATCH http://h.test/ -d a=1 -d b=2`,
			wantRaw: "PATCH / HTTP/1.1\r\nHost: h.test\r\n" +
				"Content-Type: application/x-www-form-urlencoded\r\nContent-Length: 7\r\n\r\na=1&b=2",
			wantHost: "h.test", wantPort: 80,
		},
		{
			name:     "-G moves data into the query",
			cmd:      `curl -G https://h.test/s?q=1 --data-urlencode "term=a b&c"`,
			wantRaw:  "GET /s?q=1&term=a+b%26c HTTP/1.1\r\nHost: h.test\r\n\r\n",
			wantHost: "h.test", wantPort: 443, wantTLS: true,
		},
		{
			name: "user, agent, referer; flag cluster -sSLk ignored",
			cmd:  `curl -sSLk -u admin:pw -A ua/1 -e https://ref.test/ https://h.test/`,
			wantRaw: "GET / HTTP/1.1\r\nHost: h.test\r\nAuthorization: Basic YWRtaW46cHc=\r\n" +
				"User-Agent: ua/1\r\nReferer: https://ref.test/\r\n\r\n",
			wantHost: "h.test", wantPort: 443, wantTLS: true,
		},
		{
			name:     "-I is HEAD; Name; sends empty header; Name: removes a default",
			cmd:      `curl -I https://h.test/ -H 'X-Empty;' -H 'Content-Type:'`,
			wantRaw:  "HEAD / HTTP/1.1\r\nHost: h.test\r\nX-Empty: \r\n\r\n",
			wantHost: "h.test", wantPort: 443, wantTLS: true,
		},
		{
			name:     "explicit Host header wins",
			cmd:      `curl http://10.0.0.5/ -H 'Host: vhost.test'`,
			wantRaw:  "GET / HTTP/1.1\r\nHost: vhost.test\r\n\r\n",
			wantHost: "10.0.0.5", wantPort: 80,
		},
		{
			name:     "--url and IPv6 host",
			cmd:      `curl --url 'http://[::1]:8080/p'`,
			wantRaw:  "GET /p HTTP/1.1\r\nHost: [::1]:8080\r\n\r\n",
			wantHost: "::1", wantPort: 8080,
		},
		{
			name:     "empty -d still sends Content-Length 0 as POST",
			cmd:      `curl https://h.test/ -d ''`,
			wantRaw:  "POST / HTTP/1.1\r\nHost: h.test\r\nContent-Type: application/x-www-form-urlencoded\r\nContent-Length: 0\r\n\r\n",
			wantHost: "h.test", wantPort: 443, wantTLS: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseCurl(tt.cmd)
			if err != nil {
				t.Fatalf("ParseCurl: %v", err)
			}
			if got.Raw != tt.wantRaw {
				t.Errorf("raw:\n got %q\nwant %q", got.Raw, tt.wantRaw)
			}
			if got.Host != tt.wantHost || got.Port != tt.wantPort || got.TLS != tt.wantTLS {
				t.Errorf("target: got %s:%d tls=%v, want %s:%d tls=%v",
					got.Host, got.Port, got.TLS, tt.wantHost, tt.wantPort, tt.wantTLS)
			}
		})
	}
}

func TestParseCurlRejects(t *testing.T) {
	tests := []struct {
		name, cmd, wantErr string
	}{
		{"not curl", `wget https://h.test/`, "not a curl command"},
		{"no URL", `curl -H 'A: b'`, "no URL"},
		{"two URLs", `curl https://a.test/ https://b.test/`, "more than one URL"},
		{"unknown option", `curl --frobnicate https://h.test/`, "unsupported curl option"},
		{"data from file", `curl -d @body.json https://h.test/`, "@file"},
		{"urlencode from file", `curl --data-urlencode name@file https://h.test/`, "@file"},
		{"multipart form", `curl -F a=b https://h.test/`, "multipart"},
		{"cookie file", `curl -b cookies.txt https://h.test/`, "cookie file"},
		{"bad scheme", `curl ftp://h.test/`, "scheme"},
		{"missing value", `curl https://h.test/ -H`, "needs a value"},
		{"unterminated quote", `curl 'https://h.test/`, "unterminated"},
		{"bad header", `curl https://h.test/ -H 'novalue'`, "invalid header"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseCurl(tt.cmd)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("want error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}
