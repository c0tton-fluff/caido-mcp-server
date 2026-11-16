package auth

import (
	"context"
	"strings"
	"testing"
	"time"

	caido "github.com/caido-community/sdk-go"
)

func testClient(t *testing.T, url string) *caido.Client {
	t.Helper()
	client, err := caido.NewClient(caido.Options{URL: url})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return client
}

// TestStaticAccessToken asserts the environment variables are read in order,
// including the deprecated fallback.
func TestStaticAccessToken(t *testing.T) {
	t.Setenv("CAIDO_ACCESS_TOKEN", "access")
	t.Setenv("CAIDO_PAT", "pat")
	if got := StaticAccessToken(); got != "access" {
		t.Errorf("StaticAccessToken() = %q, want access", got)
	}

	t.Setenv("CAIDO_ACCESS_TOKEN", "")
	if got := StaticAccessToken(); got != "pat" {
		t.Errorf("StaticAccessToken() = %q, want the CAIDO_PAT fallback", got)
	}

	t.Setenv("CAIDO_PAT", "")
	if got := StaticAccessToken(); got != "" {
		t.Errorf("StaticAccessToken() = %q, want empty", got)
	}
}

// TestResolveAccessToken_Static asserts a configured static token wins without
// touching the filesystem.
func TestResolveAccessToken_Static(t *testing.T) {
	t.Setenv("CAIDO_ACCESS_TOKEN", "static-token")
	t.Setenv("CAIDO_PAT", "")
	t.Setenv("HOME", t.TempDir())

	token, err := ResolveAccessToken(
		context.Background(), testClient(t, "http://127.0.0.1:1"),
	)
	if err != nil {
		t.Fatalf("ResolveAccessToken: %v", err)
	}
	if token != "static-token" {
		t.Errorf("token = %q, want static-token", token)
	}
}

// TestResolveAccessToken_Stored asserts a valid stored login is returned for
// the client's instance and that another instance's token is not.
func TestResolveAccessToken_Stored(t *testing.T) {
	t.Setenv("CAIDO_ACCESS_TOKEN", "")
	t.Setenv("CAIDO_PAT", "")
	t.Setenv("HOME", t.TempDir())

	store, err := NewTokenStore("http://127.0.0.1:8911")
	if err != nil {
		t.Fatalf("NewTokenStore: %v", err)
	}
	if err := store.Save(&StoredToken{
		AccessToken: "stored-token",
		ExpiresAt:   time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	token, err := ResolveAccessToken(
		context.Background(), testClient(t, "http://127.0.0.1:8911"),
	)
	if err != nil {
		t.Fatalf("ResolveAccessToken: %v", err)
	}
	if token != "stored-token" {
		t.Errorf("token = %q, want stored-token", token)
	}

	// A different instance has no stored login.
	_, err = ResolveAccessToken(
		context.Background(), testClient(t, "http://127.0.0.1:9999"),
	)
	if err == nil || !strings.Contains(err.Error(), "no token configured") {
		t.Fatalf("other instance: error = %v, want a no-token error", err)
	}
}

// TestResolveAccessToken_ExpiredWithoutRefresh asserts an expired login with
// no refresh token asks for a new login instead of returning a dead token.
func TestResolveAccessToken_ExpiredWithoutRefresh(t *testing.T) {
	t.Setenv("CAIDO_ACCESS_TOKEN", "")
	t.Setenv("CAIDO_PAT", "")
	t.Setenv("HOME", t.TempDir())

	store, err := NewTokenStore("http://127.0.0.1:8911")
	if err != nil {
		t.Fatalf("NewTokenStore: %v", err)
	}
	if err := store.Save(&StoredToken{
		AccessToken: "expired-token",
		ExpiresAt:   time.Now().Add(-time.Hour),
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	_, err = ResolveAccessToken(
		context.Background(), testClient(t, "http://127.0.0.1:8911"),
	)
	if err == nil || !strings.Contains(err.Error(), "caido-mcp-server login") {
		t.Fatalf("error = %v, want a login instruction", err)
	}
}
