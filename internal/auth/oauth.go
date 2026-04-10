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

	caido "github.com/caido-community/sdk-go"
	"github.com/gorilla/websocket"
)

// Authenticator handles the OAuth authentication flow
type Authenticator struct {
	client     *caido.Client
	tokenStore *TokenStore
}

// NewAuthenticator creates a new authenticator
func NewAuthenticator(
	client *caido.Client,
) (*Authenticator, error) {
	tokenStore, err := NewTokenStore()
	if err != nil {
		return nil, err
	}

	return &Authenticator{
		client:     client,
		tokenStore: tokenStore,
	}, nil
}

// EnsureAuthenticated ensures we have a valid token.
// Returns the access token or an error.
func (a *Authenticator) EnsureAuthenticated(
	ctx context.Context,
) (string, error) {
	token, err := a.tokenStore.Load()
	if err != nil {
		return "", fmt.Errorf("failed to load token: %w", err)
	}

	if token != nil && !a.tokenStore.IsExpired(token) {
		return token.AccessToken, nil
	}

	if token != nil && token.RefreshToken != "" {
		newToken, err := a.refreshToken(
			ctx, token.RefreshToken,
		)
		if err == nil {
			return newToken.AccessToken, nil
		}
		fmt.Fprintf(
			os.Stderr,
			"Token refresh failed: %v. "+
				"Starting new auth flow...\n", err,
		)
	}

	return a.startAuthFlow(ctx)
}

// RefreshAndSave calls the Caido API to refresh an access token
// and persists the result to the token store.
func RefreshAndSave(
	ctx context.Context,
	client *caido.Client,
	tokenStore *TokenStore,
	refreshToken string,
) (*StoredToken, error) {
	resp, err := client.Auth.RefreshAuthenticationToken(
		ctx, refreshToken,
	)
	if err != nil {
		return nil, err
	}

	payload := resp.RefreshAuthenticationToken
	if payload.Error != nil || payload.Token == nil {
		return nil, fmt.Errorf("token refresh failed")
	}

	token := payload.Token
	refreshTok := ""
	if token.RefreshToken != nil {
		refreshTok = *token.RefreshToken
	}

	stored := &StoredToken{
		AccessToken:  token.AccessToken,
		RefreshToken: refreshTok,
		ExpiresAt:    ParseExpiresAt(token.ExpiresAt),
	}

	if err := tokenStore.Save(stored); err != nil {
		return nil, fmt.Errorf(
			"failed to save refreshed token: %w", err,
		)
	}

	return stored, nil
}

// refreshToken attempts to refresh the access token
func (a *Authenticator) refreshToken(
	ctx context.Context, refreshToken string,
) (*StoredToken, error) {
	return RefreshAndSave(
		ctx, a.client, a.tokenStore, refreshToken,
	)
}

// startAuthFlow initiates the OAuth authentication flow
func (a *Authenticator) startAuthFlow(
	ctx context.Context,
) (string, error) {
	resp, err := a.client.Auth.StartAuthenticationFlow(ctx)
	if err != nil {
		return "", fmt.Errorf(
			"failed to start authentication flow: %w", err,
		)
	}

	payload := resp.StartAuthenticationFlow
	if payload.Error != nil || payload.Request == nil {
		return "", fmt.Errorf(
			"authentication flow returned error",
		)
	}

	authReq := payload.Request
	expiresAt := ParseExpiresAt(authReq.ExpiresAt)

	fmt.Fprintf(
		os.Stderr,
		"\n=== Caido Authentication Required ===\n",
	)
	fmt.Fprintf(
		os.Stderr,
		"Please open the following URL in your browser:\n",
	)
	fmt.Fprintf(os.Stderr, "  %s\n\n", authReq.VerificationUrl)
	fmt.Fprintf(
		os.Stderr,
		"And enter this code: %s\n\n", authReq.UserCode,
	)
	fmt.Fprintf(
		os.Stderr,
		"Waiting for authentication (expires at %s)...\n\n",
		expiresAt.Format(time.RFC3339),
	)

	if err := openBrowser(authReq.VerificationUrl); err != nil {
		fmt.Fprintf(
			os.Stderr,
			"(Could not open browser automatically)\n",
		)
	}

	stored, err := a.waitForToken(ctx, authReq.Id)
	if err != nil {
		return "", fmt.Errorf(
			"failed to get authentication token: %w", err,
		)
	}

	if err := a.tokenStore.Save(stored); err != nil {
		return "", fmt.Errorf("failed to save token: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Authentication successful!\n\n")
	return stored.AccessToken, nil
}

// waitForToken waits for the auth token via WebSocket
func (a *Authenticator) waitForToken(
	ctx context.Context, requestID string,
) (*StoredToken, error) {
	wsEndpoint := a.client.WebSocketEndpoint()

	u, err := url.Parse(wsEndpoint)
	if err != nil {
		return nil, fmt.Errorf(
			"invalid websocket endpoint: %w", err,
		)
	}

	// sdk-go returns http/https scheme; gorilla requires ws/wss
	originScheme := "http"
	switch u.Scheme {
	case "https", "wss":
		u.Scheme = "wss"
		originScheme = "https"
	default:
		u.Scheme = "ws"
	}

	header := http.Header{}
	header.Set(
		"Origin",
		fmt.Sprintf("%s://%s", originScheme, u.Host),
	)
	header.Set(
		"Sec-WebSocket-Protocol", "graphql-transport-ws",
	)

	conn, _, err := websocket.DefaultDialer.DialContext(
		ctx, u.String(), header,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to connect to websocket: %w", err,
		)
	}
	defer conn.Close()

	initMsg := map[string]any{
		"type": "connection_init",
	}
	if err := conn.WriteJSON(initMsg); err != nil {
		return nil, fmt.Errorf("failed to send init: %w", err)
	}

	var ackResp map[string]any
	if err := conn.ReadJSON(&ackResp); err != nil {
		return nil, fmt.Errorf("failed to read ack: %w", err)
	}

	subMsg := map[string]any{
		"id":   "1",
		"type": "subscribe",
		"payload": map[string]any{
			"query": `subscription CreatedAuthenticationToken(
				$requestId: ID!
			) {
				createdAuthenticationToken(
					requestId: $requestId
				) {
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
			"variables": map[string]any{
				"requestId": requestID,
			},
		},
	}

	if err := conn.WriteJSON(subMsg); err != nil {
		return nil, fmt.Errorf(
			"failed to send subscription: %w", err,
		)
	}

	return a.readTokenFromWS(ctx, conn)
}

// readTokenFromWS reads and parses the token from the WS
func (a *Authenticator) readTokenFromWS(
	ctx context.Context, conn *websocket.Conn,
) (*StoredToken, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		var msg map[string]any
		if err := conn.ReadJSON(&msg); err != nil {
			return nil, fmt.Errorf(
				"failed to read message: %w", err,
			)
		}

		msgType, ok := msg["type"].(string)
		if !ok {
			continue
		}

		switch msgType {
		case "next":
			token, err := parseWSTokenPayload(msg)
			if err != nil {
				return nil, err
			}
			if token != nil {
				return token, nil
			}

		case "error":
			payload, _ := msg["payload"].([]any)
			return nil, fmt.Errorf(
				"subscription error: %v", payload,
			)

		case "complete":
			return nil, fmt.Errorf(
				"subscription completed without token",
			)
		}
	}
}

// parseWSTokenPayload extracts a StoredToken from a WS msg
func parseWSTokenPayload(
	msg map[string]any,
) (*StoredToken, error) {
	payload, ok := msg["payload"].(map[string]any)
	if !ok {
		return nil, nil
	}

	data, ok := payload["data"].(map[string]any)
	if !ok {
		return nil, nil
	}

	created, ok := data["createdAuthenticationToken"].(map[string]any)
	if !ok {
		return nil, nil
	}

	if errData, ok := created["error"].(map[string]any); ok && errData != nil {
		typename, _ := errData["__typename"].(string)
		return nil, fmt.Errorf(
			"authentication failed: %s", typename,
		)
	}

	tokenData, ok := created["token"].(map[string]any)
	if !ok {
		return nil, nil
	}

	accessToken, _ := tokenData["accessToken"].(string)
	refreshToken, _ := tokenData["refreshToken"].(string)
	expiresAtStr, _ := tokenData["expiresAt"].(string)

	return &StoredToken{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    ParseExpiresAt(expiresAtStr),
	}, nil
}

// ParseExpiresAt parses an RFC3339 expiration string.
// Falls back to 7 days from now on parse failure.
func ParseExpiresAt(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Now().Add(7 * 24 * time.Hour)
	}
	return t
}

// openBrowser opens the default browser to the given URL
func openBrowser(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf(
			"refused to open non-HTTP URL: %s", u.Scheme,
		)
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "linux":
		cmd = exec.Command("xdg-open", rawURL)
	case "windows":
		cmd = exec.Command(
			"rundll32",
			"url.dll,FileProtocolHandler", rawURL,
		)
	default:
		return fmt.Errorf("unsupported platform")
	}

	return cmd.Start()
}

// GetTokenStore returns the token store
func (a *Authenticator) GetTokenStore() *TokenStore {
	return a.tokenStore
}
