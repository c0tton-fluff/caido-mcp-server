package replay

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sync"
)

// SessionCookieStore tracks per-replay-session cookie jars in memory.
// Each session ID maps to an isolated http.CookieJar that follows
// RFC 6265 domain/path matching via the standard library.
type SessionCookieStore struct {
	mu   sync.Mutex
	jars map[string]http.CookieJar
}

// NewSessionCookieStore returns an empty store ready for use.
func NewSessionCookieStore() *SessionCookieStore {
	return &SessionCookieStore{jars: make(map[string]http.CookieJar)}
}

// GetJar returns the existing jar for sessionID or lazily creates one.
func (s *SessionCookieStore) GetJar(sessionID string) (http.CookieJar, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if jar, ok := s.jars[sessionID]; ok {
		return jar, nil
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("create cookie jar: %w", err)
	}
	s.jars[sessionID] = jar
	return jar, nil
}

// Cookies returns cookies that match u for sessionID. Returns nil when
// the session has no jar yet.
func (s *SessionCookieStore) Cookies(sessionID string, u *url.URL) []*http.Cookie {
	if u == nil {
		return nil
	}
	s.mu.Lock()
	jar, ok := s.jars[sessionID]
	s.mu.Unlock()
	if !ok {
		return nil
	}
	return jar.Cookies(u)
}

// SetCookies merges cookies into the jar for sessionID, creating it if
// needed. Cookies that fail RFC 6265 attribute checks are silently
// dropped by the underlying jar.
func (s *SessionCookieStore) SetCookies(
	sessionID string, u *url.URL, cookies []*http.Cookie,
) error {
	if u == nil || len(cookies) == 0 {
		return nil
	}
	jar, err := s.GetJar(sessionID)
	if err != nil {
		return err
	}
	jar.SetCookies(u, cookies)
	return nil
}

// Clear deletes the jar for sessionID. Returns true when a jar was
// removed and false when nothing was tracked for that session.
func (s *SessionCookieStore) Clear(sessionID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.jars[sessionID]; !ok {
		return false
	}
	delete(s.jars, sessionID)
	return true
}

// Copy makes to start with from's cookies. net/http/cookiejar cannot
// enumerate its contents, so the jar is shared by reference: later
// Set-Cookie on either ID is visible to both until Clear(from) drops the
// old key. A no-op when from has no jar.
func (s *SessionCookieStore) Copy(from, to string) {
	if from == to {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if jar, ok := s.jars[from]; ok {
		s.jars[to] = jar
	}
}

// Has reports whether sessionID has any tracked jar (cookies may still
// be empty if the jar was created and immediately read).
func (s *SessionCookieStore) Has(sessionID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.jars[sessionID]
	return ok
}

// defaultStore is the package-level store used by the MCP tools.
var defaultStore = NewSessionCookieStore()

// DefaultCookieStore returns the package-level store.
func DefaultCookieStore() *SessionCookieStore { return defaultStore }
