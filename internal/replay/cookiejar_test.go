package replay

import (
	"net/http"
	"net/url"
	"sync"
	"testing"
)

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	return u
}

func TestSessionCookieStore_GetJarCreatesOnce(t *testing.T) {
	s := NewSessionCookieStore()

	jar1, err := s.GetJar("session-A")
	if err != nil {
		t.Fatalf("first GetJar: %v", err)
	}
	jar2, err := s.GetJar("session-A")
	if err != nil {
		t.Fatalf("second GetJar: %v", err)
	}
	if jar1 != jar2 {
		t.Fatalf("expected same jar instance for same sessionID")
	}
}

func TestSessionCookieStore_IsolatedBetweenSessions(t *testing.T) {
	s := NewSessionCookieStore()
	u := mustURL(t, "https://example.com/")

	if err := s.SetCookies("alice", u, []*http.Cookie{
		{Name: "auth", Value: "alice-token"},
	}); err != nil {
		t.Fatalf("set alice: %v", err)
	}
	if err := s.SetCookies("bob", u, []*http.Cookie{
		{Name: "auth", Value: "bob-token"},
	}); err != nil {
		t.Fatalf("set bob: %v", err)
	}

	aliceCookies := s.Cookies("alice", u)
	bobCookies := s.Cookies("bob", u)

	if len(aliceCookies) != 1 || aliceCookies[0].Value != "alice-token" {
		t.Fatalf("alice jar mismatch: %#v", aliceCookies)
	}
	if len(bobCookies) != 1 || bobCookies[0].Value != "bob-token" {
		t.Fatalf("bob jar mismatch: %#v", bobCookies)
	}
}

func TestSessionCookieStore_ClearRemovesJar(t *testing.T) {
	s := NewSessionCookieStore()
	u := mustURL(t, "https://example.com/")

	if err := s.SetCookies("s1", u, []*http.Cookie{
		{Name: "k", Value: "v"},
	}); err != nil {
		t.Fatalf("set: %v", err)
	}
	if !s.Has("s1") {
		t.Fatalf("expected jar to exist before clear")
	}

	if !s.Clear("s1") {
		t.Fatalf("Clear should return true for existing jar")
	}
	if s.Has("s1") {
		t.Fatalf("jar should be gone after clear")
	}
	if s.Clear("s1") {
		t.Fatalf("Clear should return false for missing jar")
	}
}

func TestSessionCookieStore_RFC6265PathMatching(t *testing.T) {
	s := NewSessionCookieStore()
	u := mustURL(t, "https://example.com/admin/")
	rootURL := mustURL(t, "https://example.com/")

	cookie := &http.Cookie{
		Name:   "scoped",
		Value:  "v",
		Path:   "/admin",
		Domain: "example.com",
	}
	if err := s.SetCookies("s", u, []*http.Cookie{cookie}); err != nil {
		t.Fatalf("set: %v", err)
	}

	if got := s.Cookies("s", u); len(got) != 1 {
		t.Fatalf("expected scoped cookie at /admin/, got %d", len(got))
	}
	if got := s.Cookies("s", rootURL); len(got) != 0 {
		t.Fatalf("scoped cookie should not match /, got %d", len(got))
	}
}

func TestSessionCookieStore_ConcurrentAccess(t *testing.T) {
	s := NewSessionCookieStore()
	u := mustURL(t, "https://example.com/")

	const goroutines = 32
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			cookie := &http.Cookie{
				Name:  "c",
				Value: "v",
			}
			_ = s.SetCookies("shared", u, []*http.Cookie{cookie})
			_ = s.Cookies("shared", u)
		}(i)
	}
	wg.Wait()

	if !s.Has("shared") {
		t.Fatalf("shared jar should exist after concurrent writes")
	}
}

func TestSessionCookieStore_EmptyInputsAreNoop(t *testing.T) {
	s := NewSessionCookieStore()

	if err := s.SetCookies("s", nil, []*http.Cookie{
		{Name: "k", Value: "v"},
	}); err != nil {
		t.Fatalf("nil URL should not error: %v", err)
	}
	if s.Has("s") {
		t.Fatalf("nil URL should not create jar")
	}

	u := mustURL(t, "https://example.com/")
	if err := s.SetCookies("s", u, nil); err != nil {
		t.Fatalf("nil cookies should not error: %v", err)
	}
}

func TestSessionCookieStore_Copy(t *testing.T) {
	s := NewSessionCookieStore()
	u, _ := url.Parse("https://example.com/")
	if err := s.SetCookies("old", u, []*http.Cookie{{Name: "sid", Value: "1"}}); err != nil {
		t.Fatal(err)
	}
	s.Copy("old", "new")
	if got := s.Cookies("new", u); len(got) != 1 || got[0].Name != "sid" {
		t.Fatalf("new session should inherit sid, got %v", got)
	}
	s.Clear("old")
	if got := s.Cookies("new", u); len(got) != 1 {
		t.Fatalf("clearing the old ID must not drop the copy, got %v", got)
	}
	s.Copy("missing", "other")
	if s.Has("other") {
		t.Fatal("copying from a session with no jar must be a no-op")
	}
}
