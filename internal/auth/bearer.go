package auth

import (
	"context"
	"fmt"
	"os"

	caido "github.com/caido-community/sdk-go"
)

// StaticAccessToken returns the access token configured in the process
// environment, preferring CAIDO_ACCESS_TOKEN and falling back to the
// deprecated CAIDO_PAT. Empty means the server is using a stored login.
func StaticAccessToken() string {
	if token := os.Getenv("CAIDO_ACCESS_TOKEN"); token != "" {
		return token
	}
	return os.Getenv("CAIDO_PAT")
}

// ResolveAccessToken returns a bearer token for client's Caido instance: the
// static token when one is configured, otherwise the stored OAuth token,
// refreshed and saved when it has expired.
//
// The SDK injects this same credential into GraphQL traffic through its own
// transport. REST routes outside /graphql, such as plugin backend functions,
// need the token explicitly.
func ResolveAccessToken(
	ctx context.Context, client *caido.Client,
) (string, error) {
	if token := StaticAccessToken(); token != "" {
		return token, nil
	}

	store, err := NewTokenStore(client.BaseURL())
	if err != nil {
		return "", fmt.Errorf("resolve stored login: %w", err)
	}
	stored, err := store.Load()
	if err != nil {
		return "", fmt.Errorf("load stored login: %w", err)
	}
	if stored == nil {
		return "", fmt.Errorf(
			"no token configured: set CAIDO_ACCESS_TOKEN or run " +
				"\"caido-mcp-server login\"",
		)
	}
	if !store.IsExpired(stored) {
		return stored.AccessToken, nil
	}
	if stored.RefreshToken == "" {
		return "", fmt.Errorf(
			"access token expired and no refresh token is stored: " +
				"run \"caido-mcp-server login\"",
		)
	}

	refreshed, err := RefreshAndSave(ctx, client, store, stored.RefreshToken)
	if err != nil {
		return "", fmt.Errorf("refresh access token: %w", err)
	}
	return refreshed.AccessToken, nil
}
