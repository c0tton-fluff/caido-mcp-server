package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/c0tton-fluff/caido-mcp-server/internal/auth"
	caido "github.com/caido-community/sdk-go"
	"github.com/spf13/cobra"
)

func loadToken() (*auth.StoredToken, error) {
	store, err := auth.NewTokenStore()
	if err != nil {
		return nil, err
	}
	token, err := store.Load()
	if err != nil {
		return nil, fmt.Errorf(
			"no token found - run 'caido-mcp-server login' first: %w",
			err,
		)
	}
	if token == nil {
		return nil, fmt.Errorf(
			"no token found - run 'caido-mcp-server login' first",
		)
	}
	return token, nil
}

func getCaidoURL(cmd *cobra.Command) (string, error) {
	u, _ := cmd.Flags().GetString("url")
	if u == "" {
		u = os.Getenv("CAIDO_URL")
	}
	if u == "" {
		return "", fmt.Errorf(
			"Caido URL required: set --url or CAIDO_URL env",
		)
	}
	return u, nil
}

func newClient(cmd *cobra.Command) (*caido.Client, error) {
	url, err := getCaidoURL(cmd)
	if err != nil {
		return nil, err
	}
	tok, err := loadToken()
	if err != nil {
		return nil, err
	}

	client, err := caido.NewClient(caido.Options{URL: url})
	if err != nil {
		return nil, fmt.Errorf("client init: %w", err)
	}
	client.SetAccessToken(tok.AccessToken)

	tokenStore, err := auth.NewTokenStore()
	if err != nil {
		return nil, fmt.Errorf("token store: %w", err)
	}

	client.SetTokenRefresher(func(ctx context.Context) (string, error) {
		t, err := tokenStore.Load()
		if err != nil || t == nil {
			return "", nil
		}
		if time.Now().Add(5 * time.Minute).Before(t.ExpiresAt) {
			return t.AccessToken, nil
		}
		if t.RefreshToken == "" {
			return "", fmt.Errorf("token expired, no refresh token")
		}
		stored, err := auth.RefreshAndSave(
			ctx, client, tokenStore, t.RefreshToken,
		)
		if err != nil {
			return "", err
		}
		return stored.AccessToken, nil
	})

	return client, nil
}
