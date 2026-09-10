package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	configDirName = ".caido-mcp"
	tokensDirName = "tokens"
	// legacyTokenFileName is the single global token file used before
	// credentials were stored per server. It is migrated on first use.
	legacyTokenFileName = "token.json"
	filePermission      = 0600
	dirPermission       = 0700
	// maxLabelLen caps the human-readable part of a token file name so a
	// long host or path cannot push the name past a filesystem limit. The
	// hash suffix, not the label, is what keeps names unique.
	maxLabelLen = 48
)

// StoredToken represents the token data stored on disk
type StoredToken struct {
	AccessToken  string    `json:"accessToken"`
	RefreshToken string    `json:"refreshToken"`
	ExpiresAt    time.Time `json:"expiresAt"`
	// ServerURL records the canonical Caido instance this token belongs
	// to. It is stamped by Save and exists so a token file is
	// self-describing; loading does not depend on it.
	ServerURL string `json:"serverUrl,omitempty"`
}

// TokenStore manages token persistence for a single Caido instance.
// Each instance gets its own file under ~/.caido-mcp/tokens/, so
// switching instances does not overwrite the previous login.
type TokenStore struct {
	configDir string
	serverURL string
	fileName  string
}

// NewTokenStore creates a token store scoped to serverURL. Credentials for
// different instances never share a file, so a user can stay logged in to
// several Caido instances at once.
//
// As a side effect, a pre-existing global ~/.caido-mcp/token.json is
// migrated into this server's slot on first use (see migrateLegacyToken).
func NewTokenStore(serverURL string) (*TokenStore, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	return newTokenStoreIn(filepath.Join(homeDir, configDirName), serverURL)
}

// newTokenStoreIn builds a store rooted at an explicit config directory.
// NewTokenStore is the production entry point; tests use this to avoid
// touching the real home directory while still exercising key derivation.
func newTokenStoreIn(configDir, serverURL string) (*TokenStore, error) {
	canonical, err := CanonicalServerURL(serverURL)
	if err != nil {
		return nil, err
	}

	store := &TokenStore{
		configDir: configDir,
		serverURL: canonical,
		fileName:  serverKey(canonical) + ".json",
	}

	store.migrateLegacyToken()

	return store, nil
}

// ServerURL returns the canonical instance URL this store is scoped to.
func (s *TokenStore) ServerURL() string {
	return s.serverURL
}

// tokensDir returns the directory holding the per-server token files
func (s *TokenStore) tokensDir() string {
	return filepath.Join(s.configDir, tokensDirName)
}

// tokenFilePath returns the full path to this server's token file
func (s *TokenStore) tokenFilePath() string {
	return filepath.Join(s.tokensDir(), s.fileName)
}

// legacyTokenFilePath returns the path of the pre-per-server token file
func (s *TokenStore) legacyTokenFilePath() string {
	return filepath.Join(s.configDir, legacyTokenFileName)
}

// ensureTokensDir creates the tokens directory if it doesn't exist
func (s *TokenStore) ensureTokensDir() error {
	if err := os.MkdirAll(s.tokensDir(), dirPermission); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	return nil
}

// migrateLegacyToken adopts a pre-existing global token file into this
// server's slot, so upgrading users are not forced to log in again.
//
// It runs only when this server has no token of its own, and it removes the
// legacy file so a second instance cannot also claim the same credential.
// The first instance used after upgrading therefore inherits the login; if
// that is not the instance the token was actually issued for, the token
// simply fails to validate and the normal login flow takes over.
//
// Every failure here is non-fatal: migration is a convenience, and a
// corrupt or unreadable legacy file must not block a fresh login. It is
// left in place in that case rather than destroyed.
func (s *TokenStore) migrateLegacyToken() {
	if _, err := os.Stat(s.tokenFilePath()); err == nil {
		return // this server already has its own token
	}

	data, err := os.ReadFile(s.legacyTokenFilePath())
	if err != nil {
		return // no legacy token, or it is unreadable
	}

	var token StoredToken
	if err := json.Unmarshal(data, &token); err != nil {
		return
	}

	if err := s.Save(&token); err != nil {
		return
	}
	if err := os.Remove(s.legacyTokenFilePath()); err != nil {
		return
	}

	// stderr only: stdout is the MCP JSON-RPC channel.
	fmt.Fprintf(
		os.Stderr,
		"note: migrated %s to per-server storage for %s\n",
		filepath.Join("~", configDirName, legacyTokenFileName),
		s.serverURL,
	)
}

// Save stores the token to disk
func (s *TokenStore) Save(token *StoredToken) error {
	if token == nil {
		return fmt.Errorf("cannot save a nil token")
	}

	if err := s.ensureTokensDir(); err != nil {
		return err
	}

	// Copy before stamping so Save never mutates the caller's token.
	record := *token
	record.ServerURL = s.serverURL

	data, err := json.MarshalIndent(&record, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal token: %w", err)
	}

	tmpPath := s.tokenFilePath() + ".tmp"
	if err := os.WriteFile(tmpPath, data, filePermission); err != nil {
		return fmt.Errorf("failed to write token file: %w", err)
	}
	if err := os.Rename(tmpPath, s.tokenFilePath()); err != nil {
		_ = os.Remove(tmpPath) // best-effort cleanup of the temp file
		return fmt.Errorf("failed to rename token file: %w", err)
	}

	return nil
}

// Load retrieves the token from disk
func (s *TokenStore) Load() (*StoredToken, error) {
	data, err := os.ReadFile(s.tokenFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No token stored
		}
		return nil, fmt.Errorf("failed to read token file: %w", err)
	}

	var token StoredToken
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, fmt.Errorf("failed to unmarshal token: %w", err)
	}

	return &token, nil
}

// Delete removes the token file
func (s *TokenStore) Delete() error {
	err := os.Remove(s.tokenFilePath())
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete token file: %w", err)
	}
	return nil
}

// IsExpired checks if the token is expired or about to expire
func (s *TokenStore) IsExpired(token *StoredToken) bool {
	if token == nil {
		return true
	}
	// Consider expired if less than 5 minutes remaining
	return time.Now().Add(5 * time.Minute).After(token.ExpiresAt)
}

// CanonicalServerURL normalizes a Caido instance URL into the stable form
// used to scope stored credentials. Spellings that address the same
// instance collapse to one key: the scheme and host are lowercased, a
// default port (:80 for http, :443 for https) is dropped, a trailing slash
// is removed, and credentials, query and fragment are discarded. A bare
// host:port with no scheme is read as http.
func CanonicalServerURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("caido server URL is required")
	}

	if !strings.Contains(trimmed, "://") {
		trimmed = "http://" + trimmed
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid caido server URL %q: %w", raw, err)
	}

	scheme := strings.ToLower(parsed.Scheme)
	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return "", fmt.Errorf("caido server URL %q has no host", raw)
	}

	port := parsed.Port()
	if (scheme == "http" && port == "80") ||
		(scheme == "https" && port == "443") {
		port = ""
	}

	// Hostname() strips IPv6 brackets; restore them so the result stays a
	// parseable URL when there is no port for JoinHostPort to re-add.
	hostPort := host
	switch {
	case port != "":
		hostPort = net.JoinHostPort(host, port)
	case strings.Contains(host, ":"):
		hostPort = "[" + host + "]"
	}

	path := strings.TrimRight(parsed.EscapedPath(), "/")

	return scheme + "://" + hostPort + path, nil
}

// serverKey derives the token file name for a canonical server URL. The
// readable label makes the directory inspectable; the hash suffix is what
// guarantees two different instances never collide after sanitization.
func serverKey(canonicalURL string) string {
	sum := sha256.Sum256([]byte(canonicalURL))
	return sanitizeLabel(canonicalURL) + "-" + hex.EncodeToString(sum[:4])
}

// sanitizeLabel reduces a canonical URL to a short filesystem-safe label.
func sanitizeLabel(canonicalURL string) string {
	label := canonicalURL
	if _, after, found := strings.Cut(label, "://"); found {
		label = after
	}

	var b strings.Builder
	for _, r := range label {
		switch {
		case r >= 'a' && r <= 'z',
			r >= '0' && r <= '9',
			r == '.', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}

	trimmed := strings.Trim(b.String(), "_")
	if trimmed == "" {
		trimmed = "server"
	}
	if len(trimmed) > maxLabelLen {
		trimmed = strings.Trim(trimmed[:maxLabelLen], "_")
	}

	return trimmed
}
