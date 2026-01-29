package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	configDirName  = ".caido-mcp"
	tokenFileName  = "token.json"
	filePermission = 0600
	dirPermission  = 0700
)

// StoredToken represents the token data stored on disk
type StoredToken struct {
	AccessToken  string    `json:"accessToken"`
	RefreshToken string    `json:"refreshToken"`
	ExpiresAt    time.Time `json:"expiresAt"`
}

// TokenStore manages token persistence
type TokenStore struct {
	configDir string
}

// NewTokenStore creates a new token store
func NewTokenStore() (*TokenStore, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, configDirName)
	return &TokenStore{configDir: configDir}, nil
}

// tokenFilePath returns the full path to the token file
func (s *TokenStore) tokenFilePath() string {
	return filepath.Join(s.configDir, tokenFileName)
}

// ensureConfigDir creates the config directory if it doesn't exist
func (s *TokenStore) ensureConfigDir() error {
	if err := os.MkdirAll(s.configDir, dirPermission); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	return nil
}

// Save stores the token to disk
func (s *TokenStore) Save(token *StoredToken) error {
	if err := s.ensureConfigDir(); err != nil {
		return err
	}

	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal token: %w", err)
	}

	if err := os.WriteFile(s.tokenFilePath(), data, filePermission); err != nil {
		return fmt.Errorf("failed to write token file: %w", err)
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
