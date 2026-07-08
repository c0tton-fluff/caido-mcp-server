package main

import "testing"

func TestBuildRequest(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		url      string
		headers  []string
		body     string
		wantRaw  string
		wantHost string
		wantPort int
		wantTLS  bool
	}{
		{
			name:     "https default port omits port in Host",
			method:   "GET",
			url:      "https://example.com/api",
			wantRaw:  "GET /api HTTP/1.1\r\nHost: example.com\r\nConnection: close\r\n\r\n",
			wantHost: "example.com",
			wantPort: 443,
			wantTLS:  true,
		},
		{
			name:     "http default port omits port in Host",
			method:   "GET",
			url:      "http://example.com/",
			wantRaw:  "GET / HTTP/1.1\r\nHost: example.com\r\nConnection: close\r\n\r\n",
			wantHost: "example.com",
			wantPort: 80,
			wantTLS:  false,
		},
		{
			name:     "non-default https port included in Host",
			method:   "GET",
			url:      "https://example.com:8443/x",
			wantRaw:  "GET /x HTTP/1.1\r\nHost: example.com:8443\r\nConnection: close\r\n\r\n",
			wantHost: "example.com",
			wantPort: 8443,
			wantTLS:  true,
		},
		{
			name:     "non-default http port included in Host",
			method:   "GET",
			url:      "http://example.com:8080/y",
			wantRaw:  "GET /y HTTP/1.1\r\nHost: example.com:8080\r\nConnection: close\r\n\r\n",
			wantHost: "example.com",
			wantPort: 8080,
			wantTLS:  false,
		},
		{
			name:     "body adds Content-Length when absent",
			method:   "POST",
			url:      "https://example.com/api",
			body:     "hello",
			wantRaw:  "POST /api HTTP/1.1\r\nHost: example.com\r\nConnection: close\r\nContent-Length: 5\r\n\r\nhello",
			wantHost: "example.com",
			wantPort: 443,
			wantTLS:  true,
		},
		{
			name:     "provided Host header suppresses auto Host",
			method:   "GET",
			url:      "https://example.com/",
			headers:  []string{"Host: custom.com"},
			wantRaw:  "GET / HTTP/1.1\r\nConnection: close\r\nHost: custom.com\r\n\r\n",
			wantHost: "example.com",
			wantPort: 443,
			wantTLS:  true,
		},
		{
			name:     "provided Connection header suppresses auto Connection",
			method:   "GET",
			url:      "https://example.com/",
			headers:  []string{"Connection: keep-alive"},
			wantRaw:  "GET / HTTP/1.1\r\nHost: example.com\r\nConnection: keep-alive\r\n\r\n",
			wantHost: "example.com",
			wantPort: 443,
			wantTLS:  true,
		},
		{
			name:     "provided Content-Length header suppresses auto Content-Length",
			method:   "POST",
			url:      "https://example.com/api",
			headers:  []string{"Content-Length: 99"},
			body:     "hello",
			wantRaw:  "POST /api HTTP/1.1\r\nHost: example.com\r\nConnection: close\r\nContent-Length: 99\r\n\r\nhello",
			wantHost: "example.com",
			wantPort: 443,
			wantTLS:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, host, port, tls, err := buildRequest(
				tt.method, tt.url, tt.headers, tt.body,
			)
			if err != nil {
				t.Fatalf("buildRequest() unexpected error: %v", err)
			}
			if raw != tt.wantRaw {
				t.Errorf("buildRequest() raw =\n%q\nwant\n%q", raw, tt.wantRaw)
			}
			if host != tt.wantHost {
				t.Errorf("buildRequest() host = %q, want %q", host, tt.wantHost)
			}
			if port != tt.wantPort {
				t.Errorf("buildRequest() port = %d, want %d", port, tt.wantPort)
			}
			if tls != tt.wantTLS {
				t.Errorf("buildRequest() tls = %v, want %v", tls, tt.wantTLS)
			}
		})
	}
}

func TestBuildRequestNoHost(t *testing.T) {
	_, _, _, _, err := buildRequest("GET", "/just/a/path", nil, "")
	if err == nil {
		t.Fatal("buildRequest() expected error for URL with no host, got nil")
	}
}
