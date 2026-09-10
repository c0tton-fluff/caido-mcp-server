package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newTestStore builds a store rooted at dir, failing the test on error.
func newTestStore(t *testing.T, dir, serverURL string) *TokenStore {
	t.Helper()
	store, err := newTokenStoreIn(dir, serverURL)
	if err != nil {
		t.Fatalf("newTokenStoreIn(%q, %q): %v", dir, serverURL, err)
	}
	return store
}

func TestTokenStore_SaveLoadDeleteRoundtrip(t *testing.T) {
	// White-box: construct the store directly against a temp dir so the
	// test never touches the real home directory.
	store := newTestStore(t, t.TempDir(), "http://127.0.0.1:8080")

	// Load on an empty store returns (nil, nil), not an error.
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load on empty store: unexpected error: %v", err)
	}
	if loaded != nil {
		t.Fatalf("Load on empty store = %+v, want nil", loaded)
	}

	want := &StoredToken{
		AccessToken:  "access-token-123",
		RefreshToken: "refresh-token-456",
		ExpiresAt:    time.Date(2030, 6, 15, 12, 30, 45, 0, time.UTC),
	}

	if err := store.Save(want); err != nil {
		t.Fatalf("Save: unexpected error: %v", err)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("Load after Save = nil, want a token")
		return
	}
	if got.AccessToken != want.AccessToken {
		t.Errorf("AccessToken = %q, want %q", got.AccessToken, want.AccessToken)
	}
	if got.RefreshToken != want.RefreshToken {
		t.Errorf(
			"RefreshToken = %q, want %q", got.RefreshToken, want.RefreshToken,
		)
	}
	if !got.ExpiresAt.Equal(want.ExpiresAt) {
		t.Errorf("ExpiresAt = %v, want %v", got.ExpiresAt, want.ExpiresAt)
	}

	if err := store.Delete(); err != nil {
		t.Fatalf("Delete: unexpected error: %v", err)
	}

	// After Delete the token file is gone: Load is (nil, nil) again.
	afterDelete, err := store.Load()
	if err != nil {
		t.Fatalf("Load after Delete: unexpected error: %v", err)
	}
	if afterDelete != nil {
		t.Fatalf("Load after Delete = %+v, want nil", afterDelete)
	}

	// Delete on an already-absent file is a no-op, not an error.
	if err := store.Delete(); err != nil {
		t.Fatalf("Delete on absent file: unexpected error: %v", err)
	}
}

func TestTokenStore_SaveWritesRestrictivePermissions(t *testing.T) {
	store := newTestStore(t, t.TempDir(), "http://127.0.0.1:8080")

	if err := store.Save(&StoredToken{AccessToken: "x"}); err != nil {
		t.Fatalf("Save: unexpected error: %v", err)
	}

	info, err := os.Stat(store.tokenFilePath())
	if err != nil {
		t.Fatalf("stat token file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != filePermission {
		t.Errorf("token file mode = %o, want %o", perm, filePermission)
	}
}

// The point of the issue: a second instance must not evict the first.
func TestTokenStore_DistinctServersDoNotShareCredentials(t *testing.T) {
	dir := t.TempDir()

	first := newTestStore(t, dir, "http://127.0.0.1:8080")
	second := newTestStore(t, dir, "http://127.0.0.1:9090")

	if first.tokenFilePath() == second.tokenFilePath() {
		t.Fatalf(
			"distinct servers share token file %q", first.tokenFilePath(),
		)
	}

	if err := first.Save(&StoredToken{AccessToken: "first-token"}); err != nil {
		t.Fatalf("Save first: %v", err)
	}
	if err := second.Save(&StoredToken{AccessToken: "second-token"}); err != nil {
		t.Fatalf("Save second: %v", err)
	}

	// Saving the second server's token must leave the first intact.
	got, err := first.Load()
	if err != nil || got == nil {
		t.Fatalf("Load first: token=%v err=%v", got, err)
	}
	if got.AccessToken != "first-token" {
		t.Errorf(
			"first server AccessToken = %q, want %q",
			got.AccessToken, "first-token",
		)
	}

	// And a third instance that was never logged in sees no token, rather
	// than inheriting one of the other two.
	third := newTestStore(t, dir, "https://caido.example.com")
	unseen, err := third.Load()
	if err != nil {
		t.Fatalf("Load third: %v", err)
	}
	if unseen != nil {
		t.Errorf("unlogged server Load = %+v, want nil", unseen)
	}
}

func TestTokenStore_SaveStampsServerURLWithoutMutatingCaller(t *testing.T) {
	store := newTestStore(t, t.TempDir(), "HTTP://LocalHost:8080/")

	token := &StoredToken{AccessToken: "x"}
	if err := store.Save(token); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if token.ServerURL != "" {
		t.Errorf(
			"Save mutated caller's token ServerURL = %q, want empty",
			token.ServerURL,
		)
	}

	got, err := store.Load()
	if err != nil || got == nil {
		t.Fatalf("Load: token=%v err=%v", got, err)
	}
	if got.ServerURL != "http://localhost:8080" {
		t.Errorf(
			"stored ServerURL = %q, want %q",
			got.ServerURL, "http://localhost:8080",
		)
	}
}

func TestTokenStore_SaveNilTokenIsAnError(t *testing.T) {
	store := newTestStore(t, t.TempDir(), "http://127.0.0.1:8080")
	if err := store.Save(nil); err == nil {
		t.Error("Save(nil) = nil, want an error")
	}
}

func TestCanonicalServerURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "http://127.0.0.1:8080", "http://127.0.0.1:8080"},
		{"trailing slash", "http://127.0.0.1:8080/", "http://127.0.0.1:8080"},
		{"uppercase scheme and host", "HTTP://LocalHost:8080",
			"http://localhost:8080"},
		{"default http port dropped", "http://caido.test:80",
			"http://caido.test"},
		{"default https port dropped", "https://caido.test:443",
			"https://caido.test"},
		{"non-default port kept", "https://caido.test:8443",
			"https://caido.test:8443"},
		{"no scheme means http", "127.0.0.1:8080", "http://127.0.0.1:8080"},
		{"query and fragment dropped", "http://caido.test:8080/?a=1#frag",
			"http://caido.test:8080"},
		{"userinfo dropped", "http://user:pass@caido.test:8080",
			"http://caido.test:8080"},
		{"surrounding whitespace", "  http://caido.test:8080  ",
			"http://caido.test:8080"},
		{"subpath kept", "https://caido.test/instance-a/",
			"https://caido.test/instance-a"},
		{"ipv6 with port", "http://[::1]:8080", "http://[::1]:8080"},
		{"ipv6 without port", "http://[::1]", "http://[::1]"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CanonicalServerURL(tc.in)
			if err != nil {
				t.Fatalf("CanonicalServerURL(%q): unexpected error: %v",
					tc.in, err)
			}
			if got != tc.want {
				t.Errorf("CanonicalServerURL(%q) = %q, want %q",
					tc.in, got, tc.want)
			}
		})
	}
}

func TestCanonicalServerURL_Rejects(t *testing.T) {
	for _, in := range []string{"", "   ", "http://", "https://"} {
		if got, err := CanonicalServerURL(in); err == nil {
			t.Errorf("CanonicalServerURL(%q) = %q, want an error", in, got)
		}
	}
}

// Equivalent spellings must land in the same file, or a user who types the
// URL slightly differently is silently asked to log in again.
func TestServerKey_EquivalentURLsShareOneKey(t *testing.T) {
	groups := [][]string{
		{
			"http://127.0.0.1:8080",
			"http://127.0.0.1:8080/",
			"HTTP://127.0.0.1:8080",
			"127.0.0.1:8080",
			"http://127.0.0.1:8080/?x=1",
		},
		{
			"https://caido.test",
			"https://caido.test:443/",
			"https://CAIDO.test",
		},
	}

	for _, group := range groups {
		var want string
		for i, raw := range group {
			canonical, err := CanonicalServerURL(raw)
			if err != nil {
				t.Fatalf("CanonicalServerURL(%q): %v", raw, err)
			}
			key := serverKey(canonical)
			if i == 0 {
				want = key
				continue
			}
			if key != want {
				t.Errorf(
					"serverKey(%q) = %q, want %q (same instance as %q)",
					raw, key, want, group[0],
				)
			}
		}
	}
}

// Sanitization maps many characters to '_', so the hash suffix is what has
// to keep distinct instances apart.
func TestServerKey_DistinctURLsDoNotCollide(t *testing.T) {
	urls := []string{
		"http://caido-a.test:8080",
		"http://caido_a.test:8080",
		"http://caido.a.test:8080",
		"http://caido-a.test:8081",
		"https://caido-a.test:8080",
		"https://caido.test/instance-a",
		"https://caido.test/instance-b",
	}

	seen := make(map[string]string, len(urls))
	for _, raw := range urls {
		canonical, err := CanonicalServerURL(raw)
		if err != nil {
			t.Fatalf("CanonicalServerURL(%q): %v", raw, err)
		}
		key := serverKey(canonical)
		if prev, dup := seen[key]; dup {
			t.Errorf("key %q collides for %q and %q", key, prev, raw)
			continue
		}
		seen[key] = raw
	}
}

func TestSanitizeLabel_BoundsFileNameLength(t *testing.T) {
	long := "https://" + string(make([]byte, 200)) + "caido.test"
	label := sanitizeLabel(long)
	if len(label) > maxLabelLen {
		t.Errorf("label length = %d, want <= %d", len(label), maxLabelLen)
	}
	if label == "" {
		t.Error("label is empty, want a non-empty fallback")
	}
}

// Upgrading users must not be forced to re-authenticate.
func TestTokenStore_MigratesLegacyGlobalToken(t *testing.T) {
	dir := t.TempDir()

	legacy := &StoredToken{
		AccessToken:  "legacy-access",
		RefreshToken: "legacy-refresh",
		ExpiresAt:    time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC),
	}
	writeLegacyToken(t, dir, legacy)

	store := newTestStore(t, dir, "http://127.0.0.1:8080")

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load after migration: %v", err)
	}
	if got == nil {
		t.Fatal("Load after migration = nil, want the migrated token")
		return
	}
	if got.AccessToken != legacy.AccessToken {
		t.Errorf("AccessToken = %q, want %q",
			got.AccessToken, legacy.AccessToken)
	}
	if got.RefreshToken != legacy.RefreshToken {
		t.Errorf("RefreshToken = %q, want %q",
			got.RefreshToken, legacy.RefreshToken)
	}
	if !got.ExpiresAt.Equal(legacy.ExpiresAt) {
		t.Errorf("ExpiresAt = %v, want %v", got.ExpiresAt, legacy.ExpiresAt)
	}
	if got.ServerURL != "http://127.0.0.1:8080" {
		t.Errorf("migrated ServerURL = %q, want %q",
			got.ServerURL, "http://127.0.0.1:8080")
	}

	// The legacy file is consumed, so a second instance cannot also claim
	// the same credential.
	if _, err := os.Stat(filepath.Join(dir, legacyTokenFileName)); !os.IsNotExist(err) {
		t.Errorf("legacy token file still present after migration (err=%v)", err)
	}

	other := newTestStore(t, dir, "http://127.0.0.1:9090")
	stolen, err := other.Load()
	if err != nil {
		t.Fatalf("Load other server: %v", err)
	}
	if stolen != nil {
		t.Errorf("second server inherited migrated token %+v, want nil", stolen)
	}
}

func TestTokenStore_MigrationDoesNotOverwriteExistingToken(t *testing.T) {
	dir := t.TempDir()

	// A store that already has its own token for this instance.
	store := newTestStore(t, dir, "http://127.0.0.1:8080")
	if err := store.Save(&StoredToken{AccessToken: "current"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	writeLegacyToken(t, dir, &StoredToken{AccessToken: "legacy"})

	// Re-constructing the store must not let the legacy file win.
	reopened := newTestStore(t, dir, "http://127.0.0.1:8080")
	got, err := reopened.Load()
	if err != nil || got == nil {
		t.Fatalf("Load: token=%v err=%v", got, err)
	}
	if got.AccessToken != "current" {
		t.Errorf("AccessToken = %q, want %q", got.AccessToken, "current")
	}
}

// A corrupt legacy file must not block a fresh login, and must not be
// destroyed either.
func TestTokenStore_CorruptLegacyTokenIsLeftAlone(t *testing.T) {
	dir := t.TempDir()

	legacyPath := filepath.Join(dir, legacyTokenFileName)
	if err := os.MkdirAll(dir, dirPermission); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(legacyPath, []byte("{not json"), filePermission); err != nil {
		t.Fatalf("write legacy token: %v", err)
	}

	store := newTestStore(t, dir, "http://127.0.0.1:8080")

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != nil {
		t.Errorf("Load = %+v, want nil for a corrupt legacy file", got)
	}
	if _, err := os.Stat(legacyPath); err != nil {
		t.Errorf("corrupt legacy file was removed: %v", err)
	}
}

func TestTokenStore_NoLegacyTokenIsNotAnError(t *testing.T) {
	store := newTestStore(t, t.TempDir(), "http://127.0.0.1:8080")
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != nil {
		t.Errorf("Load = %+v, want nil", got)
	}
}

func TestNewTokenStore_RejectsEmptyServerURL(t *testing.T) {
	if _, err := NewTokenStore(""); err == nil {
		t.Error("NewTokenStore(\"\") = nil error, want an error")
	}
}

func writeLegacyToken(t *testing.T, dir string, token *StoredToken) {
	t.Helper()
	if err := os.MkdirAll(dir, dirPermission); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		t.Fatalf("marshal legacy token: %v", err)
	}
	path := filepath.Join(dir, legacyTokenFileName)
	if err := os.WriteFile(path, data, filePermission); err != nil {
		t.Fatalf("write legacy token: %v", err)
	}
}
