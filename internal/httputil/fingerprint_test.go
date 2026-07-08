package httputil

import (
	"testing"
)

func TestFingerprintFromHeaders(t *testing.T) {
	tests := []struct {
		name     string
		headers  []Header
		bodySize int
		wantKind ContentKind
	}{
		{
			name:     "json content type",
			headers:  []Header{{Name: "Content-Type", Value: "application/json; charset=utf-8"}},
			bodySize: 500,
			wantKind: KindJSON,
		},
		{
			name:     "html content type",
			headers:  []Header{{Name: "Content-Type", Value: "text/html"}},
			bodySize: 1000,
			wantKind: KindHTML,
		},
		{
			name:     "binary image",
			headers:  []Header{{Name: "Content-Type", Value: "image/png"}},
			bodySize: 50000,
			wantKind: KindBinary,
		},
		{
			name:     "pdf binary",
			headers:  []Header{{Name: "Content-Type", Value: "application/pdf"}},
			bodySize: 100000,
			wantKind: KindBinary,
		},
		{
			name:     "empty body",
			headers:  []Header{{Name: "Content-Type", Value: "text/html"}},
			bodySize: 0,
			wantKind: KindEmpty,
		},
		{
			name:     "xml content type",
			headers:  []Header{{Name: "Content-Type", Value: "application/xml"}},
			bodySize: 200,
			wantKind: KindXML,
		},
		{
			name:     "javascript",
			headers:  []Header{{Name: "Content-Type", Value: "application/javascript"}},
			bodySize: 300,
			wantKind: KindText,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fp := FingerprintFromHeaders(tt.headers, tt.bodySize)
			if fp.Kind != tt.wantKind {
				t.Fatalf("want kind %q, got %q", tt.wantKind, fp.Kind)
			}
		})
	}
}

func TestFingerprintFromBody(t *testing.T) {
	tests := []struct {
		name     string
		body     []byte
		wantKind ContentKind
	}{
		{
			name:     "json object",
			body:     []byte(`{"status": "ok"}`),
			wantKind: KindJSON,
		},
		{
			name:     "json array",
			body:     []byte(`[{"id": 1}]`),
			wantKind: KindJSON,
		},
		{
			name:     "html document",
			body:     []byte(`<!DOCTYPE html><html><body>hi</body></html>`),
			wantKind: KindHTML,
		},
		{
			name:     "xml document",
			body:     []byte(`<?xml version="1.0"?><root/>`),
			wantKind: KindXML,
		},
		{
			name:     "binary data",
			body:     []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10},
			wantKind: KindBinary,
		},
		{
			name:     "plain text",
			body:     []byte("hello world"),
			wantKind: KindText,
		},
		{
			name:     "empty",
			body:     []byte{},
			wantKind: KindEmpty,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fp := FingerprintFromBody(tt.body)
			if fp.Kind != tt.wantKind {
				t.Fatalf("want kind %q, got %q", tt.wantKind, fp.Kind)
			}
		})
	}
}

func TestExtractTitle(t *testing.T) {
	tests := []struct {
		name string
		body []byte
		want string
	}{
		{
			name: "simple title",
			body: []byte(`<!DOCTYPE html><html><head><title>Login Page</title></head><body></body></html>`),
			want: "Login Page",
		},
		{
			name: "title with attributes and mixed case tag",
			body: []byte(`<HTML><HEAD><TiTle class="x">  Spaced Title  </TiTle></HEAD></HTML>`),
			want: "Spaced Title",
		},
		{
			name: "no title tag",
			body: []byte(`<html><body>no title here</body></html>`),
			want: "",
		},
		{
			name: "non-html body",
			body: []byte(`{"status": "ok"}`),
			want: "",
		},
		{
			name: "empty body",
			body: []byte{},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractTitle(tt.body)
			if got != tt.want {
				t.Fatalf("want %q, got %q", tt.want, got)
			}
		})
	}
}

func TestWordCount(t *testing.T) {
	tests := []struct {
		name string
		body []byte
		want int
	}{
		{"simple sentence", []byte("hello world foo"), 3},
		{"extra whitespace", []byte("  hello   world  \n\tfoo "), 3},
		{"empty", []byte{}, 0},
		{"single word", []byte("hello"), 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WordCount(tt.body)
			if got != tt.want {
				t.Fatalf("want %d, got %d", tt.want, got)
			}
		})
	}
}

func TestSetCookieNames(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		want   []string
	}{
		{
			name:   "single cookie",
			values: []string{"session=abc123; Path=/; HttpOnly"},
			want:   []string{"session"},
		},
		{
			name:   "multiple cookies",
			values: []string{"session=abc123; Path=/", "csrftoken=xyz; Secure"},
			want:   []string{"session", "csrftoken"},
		},
		{
			name:   "redacted value yields no name",
			values: []string{"[REDACTED]"},
			want:   []string{},
		},
		{
			name:   "no values",
			values: nil,
			want:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SetCookieNames(tt.values)
			if len(got) != len(tt.want) {
				t.Fatalf("want %v, got %v", tt.want, got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("want %v, got %v", tt.want, got)
				}
			}
		})
	}
}

func TestPopulateResponseDetails(t *testing.T) {
	t.Run("nil fingerprint is a no-op", func(t *testing.T) {
		PopulateResponseDetails(nil, 200, nil, nil)
	})

	t.Run("html response with redirect and cookies", func(t *testing.T) {
		headers := []Header{
			{Name: "Content-Type", Value: "text/html"},
			{Name: "Location", Value: "/dashboard"},
			{Name: "Set-Cookie", Value: "session=abc123; Path=/"},
			{Name: "Set-Cookie", Value: "csrftoken=xyz"},
		}
		// WordCount is a raw whitespace split of the whole body (tags
		// included, per spec), so spaces are placed around the visible
		// text to make the expected count easy to verify by hand: the
		// opening-tag run, six words, then the closing-tag run = 8 fields.
		body := []byte(`<html><head><title>Welcome</title></head><body> hello world foo bar baz qux </body></html>`)
		fp := FingerprintFromHeaders(headers, len(body))

		PopulateResponseDetails(&fp, 302, headers, body)

		if fp.StatusCode != 302 {
			t.Fatalf("want statusCode 302, got %d", fp.StatusCode)
		}
		if fp.Title != "Welcome" {
			t.Fatalf("want title %q, got %q", "Welcome", fp.Title)
		}
		if fp.RedirectTarget != "/dashboard" {
			t.Fatalf("want redirectTarget %q, got %q", "/dashboard", fp.RedirectTarget)
		}
		if len(fp.SetCookies) != 2 || fp.SetCookies[0] != "session" || fp.SetCookies[1] != "csrftoken" {
			t.Fatalf("want setCookies [session csrftoken], got %v", fp.SetCookies)
		}
		if fp.WordCount != 8 {
			t.Fatalf("want wordCount 8, got %d", fp.WordCount)
		}
	})

	t.Run("non-html response has no title", func(t *testing.T) {
		headers := []Header{{Name: "Content-Type", Value: "application/json"}}
		body := []byte(`{"status": "ok"}`)
		fp := FingerprintFromHeaders(headers, len(body))

		PopulateResponseDetails(&fp, 200, headers, body)

		if fp.Title != "" {
			t.Fatalf("want empty title for non-html, got %q", fp.Title)
		}
		if fp.StatusCode != 200 {
			t.Fatalf("want statusCode 200, got %d", fp.StatusCode)
		}
	})
}

func TestAdaptiveBodyLimit(t *testing.T) {
	tests := []struct {
		name      string
		kind      ContentKind
		requested int
		want      int
	}{
		{"json default", KindJSON, 0, 4000},
		{"html default", KindHTML, 0, 3000},
		{"binary default", KindBinary, 0, 200},
		{"empty", KindEmpty, 0, 0},
		{"text default", KindText, 0, DefaultBodyLimit},
		{"explicit override", KindJSON, 1000, 1000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fp := Fingerprint{Kind: tt.kind}
			got := AdaptiveBodyLimit(fp, tt.requested)
			if got != tt.want {
				t.Fatalf("want %d, got %d", tt.want, got)
			}
		})
	}
}
