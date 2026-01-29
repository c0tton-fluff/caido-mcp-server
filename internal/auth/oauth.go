package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/gorilla/websocket"
	"github.com/c0tton-fluff/caido-mcp-server/internal/caido"
)

// Authenticator handles the OAuth authentication flow
type Authenticator struct {
	client     *caido.Client
	tokenStore *TokenStore
}

// NewAuthenticator creates a new authenticator
func NewAuthenticator(client *caido.Client) (*Authenticator, error) {
	tokenStore, err := NewTokenStore()
	if err != nil {
		return nil, err
	}

	return &Authenticator{
		client:     client,
		tokenStore: tokenStore,
	}, nil
}

// EnsureAuthenticated ensures we have a valid token
// Returns the access token or an error
func (a *Authenticator) EnsureAuthenticated(ctx context.Context) (string, error) {
	// Try to load existing token
	token, err := a.tokenStore.Load()
	if err != nil {
		return "", fmt.Errorf("failed to load token: %w", err)
	}

	// If we have a valid token, use it
	if token != nil && !a.tokenStore.IsExpired(token) {
		return token.AccessToken, nil
	}

	// If we have a refresh token, try to refresh
	if token != nil && token.RefreshToken != "" {
		newToken, err := a.refreshToken(ctx, token.RefreshToken)
		if err == nil {
			return newToken.AccessToken, nil
		}
		// If refresh failed, continue to full auth flow
		fmt.Fprintf(os.Stderr, "Token refresh failed: %v. Starting new authentication flow...\n", err)
	}

	// Start full OAuth flow
	return a.startAuthFlow(ctx)
}

// refreshToken attempts to refresh the access token
func (a *Authenticator) refreshToken(ctx context.Context, refreshToken string) (*StoredToken, error) {
	token, err := a.client.RefreshToken(ctx, refreshToken)
	if err != nil {
		return nil, err
	}

	storedToken := &StoredToken{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresAt:    token.ExpiresAt,
	}

	if err := a.tokenStore.Save(storedToken); err != nil {
		return nil, fmt.Errorf("failed to save refreshed token: %w", err)
	}

	return storedToken, nil
}

// startAuthFlow initiates the OAuth authentication flow
func (a *Authenticator) startAuthFlow(ctx context.Context) (string, error) {
	// Start the authentication flow
	authReq, err := a.client.StartAuthenticationFlow(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to start authentication flow: %w", err)
	}

	// Show instructions to user (use stderr to avoid interfering with MCP protocol on stdout)
	fmt.Fprintf(os.Stderr, "\n=== Caido Authentication Required ===\n")
	fmt.Fprintf(os.Stderr, "Please open the following URL in your browser:\n")
	fmt.Fprintf(os.Stderr, "  %s\n\n", authReq.VerificationURL)
	fmt.Fprintf(os.Stderr, "And enter this code: %s\n\n", authReq.UserCode)
	fmt.Fprintf(os.Stderr, "Waiting for authentication (expires at %s)...\n\n",
		authReq.ExpiresAt.Format(time.RFC3339))

	// Try to open the browser automatically
	if err := openBrowser(authReq.VerificationURL); err != nil {
		// Not critical, user can open manually
		fmt.Fprintf(os.Stderr, "(Could not open browser automatically)\n")
	}

	// Wait for the authentication token via WebSocket
	token, err := a.waitForToken(ctx, authReq.ID)
	if err != nil {
		return "", fmt.Errorf("failed to get authentication token: %w", err)
	}

	storedToken := &StoredToken{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresAt:    token.ExpiresAt,
	}

	if err := a.tokenStore.Save(storedToken); err != nil {
		return "", fmt.Errorf("failed to save token: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Authentication successful!\n\n")
	return token.AccessToken, nil
}

// waitForToken waits for the authentication token via WebSocket subscription
func (a *Authenticator) waitForToken(ctx context.Context, requestID string) (*caido.AuthenticationToken, error) {
	wsEndpoint := a.client.WebSocketEndpoint()

	// Parse the endpoint to get the host for the Origin header
	u, err := url.Parse(wsEndpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid websocket endpoint: %w", err)
	}

	header := http.Header{}
	// Use http/https for Origin based on ws/wss
	originScheme := "http"
	if u.Scheme == "wss" {
		originScheme = "https"
	}
	header.Set("Origin", fmt.Sprintf("%s://%s", originScheme, u.Host))
	header.Set("Sec-WebSocket-Protocol", "graphql-transport-ws")

	// Connect to WebSocket
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsEndpoint, header)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to websocket: %w", err)
	}
	defer conn.Close()

	// Send connection init
	initMsg := map[string]interface{}{
		"type": "connection_init",
	}
	if err := conn.WriteJSON(initMsg); err != nil {
		return nil, fmt.Errorf("failed to send init: %w", err)
	}

	// Wait for connection_ack
	var ackResp map[string]interface{}
	if err := conn.ReadJSON(&ackResp); err != nil {
		return nil, fmt.Errorf("failed to read ack: %w", err)
	}

	// Subscribe to createdAuthenticationToken
	subID := "1"
	subMsg := map[string]interface{}{
		"id":   subID,
		"type": "subscribe",
		"payload": map[string]interface{}{
			"query": `subscription CreatedAuthenticationToken($requestId: ID!) {
				createdAuthenticationToken(requestId: $requestId) {
					token {
						accessToken
						refreshToken
						expiresAt
					}
					error {
						__typename
					}
				}
			}`,
			"variables": map[string]interface{}{
				"requestId": requestID,
			},
		},
	}

	if err := conn.WriteJSON(subMsg); err != nil {
		return nil, fmt.Errorf("failed to send subscription: %w", err)
	}

	// Wait for the token
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		var msg map[string]interface{}
		if err := conn.ReadJSON(&msg); err != nil {
			return nil, fmt.Errorf("failed to read message: %w", err)
		}

		msgType, ok := msg["type"].(string)
		if !ok {
			continue
		}

		switch msgType {
		case "next":
			payload, ok := msg["payload"].(map[string]interface{})
			if !ok {
				continue
			}

			data, ok := payload["data"].(map[string]interface{})
			if !ok {
				continue
			}

			created, ok := data["createdAuthenticationToken"].(map[string]interface{})
			if !ok {
				continue
			}

			// Check for error
			if errData, ok := created["error"].(map[string]interface{}); ok && errData != nil {
				typename, _ := errData["__typename"].(string)
				return nil, fmt.Errorf("authentication failed: %s", typename)
			}

			// Parse token
			tokenData, ok := created["token"].(map[string]interface{})
			if !ok {
				continue
			}

			accessToken, _ := tokenData["accessToken"].(string)
			refreshToken, _ := tokenData["refreshToken"].(string)
			expiresAtStr, _ := tokenData["expiresAt"].(string)

			expiresAt, err := time.Parse(time.RFC3339, expiresAtStr)
			if err != nil {
				expiresAt = time.Now().Add(7 * 24 * time.Hour) // Default to 7 days
			}

			return &caido.AuthenticationToken{
				AccessToken:  accessToken,
				RefreshToken: refreshToken,
				ExpiresAt:    expiresAt,
			}, nil

		case "error":
			payload, _ := msg["payload"].([]interface{})
			return nil, fmt.Errorf("subscription error: %v", payload)

		case "complete":
			return nil, fmt.Errorf("subscription completed without token")
		}
	}
}

// openBrowser opens the default browser to the given URL
func openBrowser(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return fmt.Errorf("unsupported platform")
	}

	return cmd.Start()
}

// GetTokenStore returns the token store
func (a *Authenticator) GetTokenStore() *TokenStore {
	return a.tokenStore
}
