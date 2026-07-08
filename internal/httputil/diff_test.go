package httputil

import (
	"testing"
)

func TestResponseCacheFirstCall(t *testing.T) {
	cache := &ResponseCache{entries: make(map[string]ResponseDigest)}
	digest := ResponseDigest{StatusCode: 200, BodyHash: 123, BodySize: 100}

	result := cache.GetAndSet("sess-1", digest)
	if result != nil {
		t.Fatal("expected nil on first call")
	}
}

func TestResponseCacheIdentical(t *testing.T) {
	cache := &ResponseCache{entries: make(map[string]ResponseDigest)}
	digest := ResponseDigest{StatusCode: 200, BodyHash: 123, BodySize: 100}

	cache.GetAndSet("sess-1", digest)
	result := cache.GetAndSet("sess-1", digest)
	if result == nil {
		t.Fatal("expected diff result")
	}
	if !result.Same {
		t.Fatal("expected same=true")
	}
}

func TestResponseCacheStatusChange(t *testing.T) {
	cache := &ResponseCache{entries: make(map[string]ResponseDigest)}

	cache.GetAndSet("sess-1", ResponseDigest{StatusCode: 200, BodyHash: 111, BodySize: 100})
	result := cache.GetAndSet("sess-1", ResponseDigest{StatusCode: 403, BodyHash: 222, BodySize: 50})
	if result == nil {
		t.Fatal("expected diff result")
	}
	if result.Same {
		t.Fatal("expected same=false")
	}
	if result.StatusChange != "200 -> 403" {
		t.Fatalf("want '200 -> 403', got %q", result.StatusChange)
	}
	if result.SizeChange != "-50 bytes" {
		t.Fatalf("want '-50 bytes', got %q", result.SizeChange)
	}
}

func TestResponseCacheBodySizeChangeSameHash(t *testing.T) {
	cache := &ResponseCache{entries: make(map[string]ResponseDigest)}

	cache.GetAndSet("sess-1", ResponseDigest{StatusCode: 200, BodyHash: 999, BodySize: 100})
	// Equal BodyHash + StatusCode but a larger BodySize: the responses share
	// the first bodyLimit bytes yet differ in total size, so they are NOT
	// identical.
	result := cache.GetAndSet("sess-1", ResponseDigest{StatusCode: 200, BodyHash: 999, BodySize: 250})
	if result == nil {
		t.Fatal("expected diff result")
	}
	if result.Same {
		t.Fatal("expected same=false when body size differs despite equal hash")
	}
	if result.SizeChange != "+150 bytes" {
		t.Fatalf("want '+150 bytes', got %q", result.SizeChange)
	}
}

func TestResponseCacheSeparateSessions(t *testing.T) {
	cache := &ResponseCache{entries: make(map[string]ResponseDigest)}
	d1 := ResponseDigest{StatusCode: 200, BodyHash: 111, BodySize: 100}
	d2 := ResponseDigest{StatusCode: 301, BodyHash: 222, BodySize: 50}

	cache.GetAndSet("sess-1", d1)
	result := cache.GetAndSet("sess-2", d2)
	if result != nil {
		t.Fatal("expected nil for new session")
	}
}

func TestHashBody(t *testing.T) {
	h1 := HashBody([]byte("hello"))
	h2 := HashBody([]byte("hello"))
	h3 := HashBody([]byte("world"))

	if h1 != h2 {
		t.Fatal("same input should produce same hash")
	}
	if h1 == h3 {
		t.Fatal("different input should produce different hash")
	}
}

func TestResponseCacheClear(t *testing.T) {
	cache := &ResponseCache{entries: make(map[string]ResponseDigest)}
	digest := ResponseDigest{StatusCode: 200, BodyHash: 123, BodySize: 100}

	cache.GetAndSet("sess-1", digest)
	cache.Clear("sess-1")
	result := cache.GetAndSet("sess-1", digest)
	if result != nil {
		t.Fatal("expected nil after clear")
	}
}
