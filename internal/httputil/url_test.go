package httputil

import "testing"

func TestBuildURL_HTTPSDefaultPort(t *testing.T) {
	got := BuildURL(true, "example.com", 443, "/path", "")
	want := "https://example.com/path"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestBuildURL_HTTPDefaultPort(t *testing.T) {
	got := BuildURL(false, "example.com", 80, "/", "")
	want := "http://example.com/"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestBuildURL_NonDefaultPort(t *testing.T) {
	got := BuildURL(false, "example.com", 8080, "/api", "")
	want := "http://example.com:8080/api"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestBuildURL_HTTPSNonDefaultPort(t *testing.T) {
	got := BuildURL(true, "example.com", 8443, "/secure", "")
	want := "https://example.com:8443/secure"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestBuildURL_WithQuery(t *testing.T) {
	got := BuildURL(false, "example.com", 80, "/search", "q=test&page=1")
	want := "http://example.com/search?q=test&page=1"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestBuildURL_EmptyQuery(t *testing.T) {
	got := BuildURL(true, "example.com", 443, "/", "")
	want := "https://example.com/"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
