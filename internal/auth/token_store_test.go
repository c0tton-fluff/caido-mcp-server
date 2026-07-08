package auth

import (
	"os"
	"testing"
	"time"
)

func TestTokenStore_SaveLoadDeleteRoundtrip(t *testing.T) {
	// White-box: construct the store directly against a temp dir so the
	// test never touches the real home directory.
	store := &TokenStore{configDir: t.TempDir()}

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
	store := &TokenStore{configDir: t.TempDir()}

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
