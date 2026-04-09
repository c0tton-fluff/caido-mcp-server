package httputil

import (
	"encoding/base64"
	"testing"
)

func TestParseBase64_EmptyInput(t *testing.T) {
	result := ParseBase64("", true, true, 0, 0)
	if result != nil {
		t.Fatal("expected nil for empty input")
	}
}

func TestParseBase64_InvalidBase64(t *testing.T) {
	result := ParseBase64("not-valid-base64!!!", true, true, 0, 0)
	if result != nil {
		t.Fatal("expected nil for invalid base64")
	}
}

func TestParseBase64_HeadersOnly(t *testing.T) {
	raw := "GET / HTTP/1.1\r\nHost: example.com\r\nAccept: */*\r\n\r\nsome body"
	encoded := base64.StdEncoding.EncodeToString([]byte(raw))

	result := ParseBase64(encoded, true, false, 0, 0)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.FirstLine != "GET / HTTP/1.1" {
		t.Fatalf("unexpected first line: %q", result.FirstLine)
	}
	if len(result.Headers) != 2 {
		t.Fatalf("expected 2 headers, got %d", len(result.Headers))
	}
	if result.Headers[0].Name != "Host" ||
		result.Headers[0].Value != "example.com" {
		t.Fatalf("unexpected header[0]: %+v", result.Headers[0])
	}
	if result.Body != "" {
		t.Fatalf("expected empty body, got %q", result.Body)
	}
	if result.BodySize != 9 {
		t.Fatalf("expected bodySize 9, got %d", result.BodySize)
	}
}

func TestParseRaw_BodyTruncation(t *testing.T) {
	raw := []byte(
		"HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\n\r\n" +
			"abcdefghij",
	)
	result := ParseRaw(raw, false, true, 0, 5)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Body != "abcde" {
		t.Fatalf("expected truncated body %q, got %q", "abcde", result.Body)
	}
	if !result.Truncated {
		t.Fatal("expected Truncated=true")
	}
	if result.BodySize != 10 {
		t.Fatalf("expected bodySize 10, got %d", result.BodySize)
	}
}

func TestParseRaw_BodyOffset(t *testing.T) {
	raw := []byte("HTTP/1.1 200 OK\r\n\r\nabcdefghij")
	result := ParseRaw(raw, false, true, 3, 0)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Body != "defghij" {
		t.Fatalf("expected body %q, got %q", "defghij", result.Body)
	}
	if result.BodySize != 10 {
		t.Fatalf("expected bodySize 10, got %d", result.BodySize)
	}
}

func TestParseRaw_BodyOffsetBeyondLength(t *testing.T) {
	raw := []byte("HTTP/1.1 200 OK\r\n\r\nabc")
	result := ParseRaw(raw, false, true, 100, 0)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Body != "" {
		t.Fatalf("expected empty body, got %q", result.Body)
	}
}

func TestParseRaw_DuplicateHeaders(t *testing.T) {
	raw := []byte(
		"HTTP/1.1 200 OK\r\n" +
			"Set-Cookie: a=1\r\n" +
			"Set-Cookie: b=2\r\n" +
			"Content-Type: text/html\r\n" +
			"\r\n",
	)
	result := ParseRaw(raw, true, false, 0, 0)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	cookieCount := 0
	for _, h := range result.Headers {
		if h.Name == "Set-Cookie" {
			cookieCount++
		}
	}
	if cookieCount != 2 {
		t.Fatalf(
			"expected 2 Set-Cookie headers, got %d; headers: %+v",
			cookieCount, result.Headers,
		)
	}
}

func TestParseRaw_SensitiveHeaderRedaction(t *testing.T) {
	raw := []byte(
		"GET / HTTP/1.1\r\n" +
			"Host: example.com\r\n" +
			"Authorization: Bearer secret-token\r\n" +
			"Cookie: session=abc123\r\n" +
			"X-Api-Key: key-456\r\n" +
			"Content-Type: text/html\r\n" +
			"\r\n",
	)
	result := ParseRaw(raw, true, false, 0, 0)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	expects := map[string]string{
		"Host":          "example.com",
		"Authorization": "[REDACTED]",
		"Cookie":        "[REDACTED]",
		"X-Api-Key":     "[REDACTED]",
		"Content-Type":  "text/html",
	}

	for _, h := range result.Headers {
		want, ok := expects[h.Name]
		if !ok {
			t.Fatalf("unexpected header: %s", h.Name)
		}
		if h.Value != want {
			t.Fatalf(
				"header %s: want %q, got %q",
				h.Name, want, h.Value,
			)
		}
	}
}

func TestParseRaw_FirstLine(t *testing.T) {
	raw := []byte("POST /api HTTP/1.1\r\nHost: test.com\r\n\r\n")
	result := ParseRaw(raw, true, false, 0, 0)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.FirstLine != "POST /api HTTP/1.1" {
		t.Fatalf("unexpected first line: %q", result.FirstLine)
	}
}
