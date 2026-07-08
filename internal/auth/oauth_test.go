package auth

import (
	"testing"
	"time"
)

func TestParseExpiresAt_ValidRFC3339(t *testing.T) {
	want := time.Date(2030, 6, 15, 12, 30, 45, 0, time.UTC)

	got := ParseExpiresAt("2030-06-15T12:30:45Z")

	if !got.Equal(want) {
		t.Fatalf("ParseExpiresAt = %v, want %v", got, want)
	}
}

func TestParseExpiresAt_FallbackOnGarbage(t *testing.T) {
	// On parse failure the function falls back to ~now+7d. Assert the
	// returned time is within a one-minute tolerance of that target so the
	// test stays deterministic despite the internal time.Now() call.
	const week = 7 * 24 * time.Hour

	cases := map[string]string{
		"garbage": "not-a-real-timestamp",
		"empty":   "",
	}

	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			got := ParseExpiresAt(input)

			delta := time.Until(got)
			if delta < week-time.Minute || delta > week+time.Minute {
				t.Fatalf(
					"ParseExpiresAt(%q) fell %v from now, want ~%v",
					input, delta, week,
				)
			}
		})
	}
}

func TestParseWSTokenPayload_Success(t *testing.T) {
	msg := map[string]any{
		"type": "next",
		"payload": map[string]any{
			"data": map[string]any{
				"createdAuthenticationToken": map[string]any{
					"token": map[string]any{
						"accessToken":  "access-abc",
						"refreshToken": "refresh-def",
						"expiresAt":    "2030-06-15T12:30:45Z",
					},
				},
			},
		},
	}

	got, err := parseWSTokenPayload(msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected a token, got nil")
	}
	if got.AccessToken != "access-abc" {
		t.Errorf("AccessToken = %q, want %q", got.AccessToken, "access-abc")
	}
	if got.RefreshToken != "refresh-def" {
		t.Errorf(
			"RefreshToken = %q, want %q", got.RefreshToken, "refresh-def",
		)
	}
	want := time.Date(2030, 6, 15, 12, 30, 45, 0, time.UTC)
	if !got.ExpiresAt.Equal(want) {
		t.Errorf("ExpiresAt = %v, want %v", got.ExpiresAt, want)
	}
}

func TestParseWSTokenPayload_ErrorTypename(t *testing.T) {
	msg := map[string]any{
		"type": "next",
		"payload": map[string]any{
			"data": map[string]any{
				"createdAuthenticationToken": map[string]any{
					"error": map[string]any{
						"__typename": "AuthenticationExpired",
					},
				},
			},
		},
	}

	got, err := parseWSTokenPayload(msg)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if got != nil {
		t.Errorf("expected nil token on error, got %+v", got)
	}
}

func TestParseWSTokenPayload_MalformedReturnsNilNil(t *testing.T) {
	cases := map[string]map[string]any{
		"missing payload": {
			"type": "next",
		},
		"payload wrong type": {
			"payload": "not-a-map",
		},
		"missing data": {
			"payload": map[string]any{},
		},
		"missing createdAuthenticationToken": {
			"payload": map[string]any{
				"data": map[string]any{},
			},
		},
		"missing token and error": {
			"payload": map[string]any{
				"data": map[string]any{
					"createdAuthenticationToken": map[string]any{},
				},
			},
		},
	}

	for name, msg := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := parseWSTokenPayload(msg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != nil {
				t.Fatalf("expected nil token, got %+v", got)
			}
		})
	}
}

func TestIsExpired(t *testing.T) {
	s := &TokenStore{}

	if !s.IsExpired(nil) {
		t.Error("IsExpired(nil) = false, want true")
	}

	past := &StoredToken{ExpiresAt: time.Now().Add(-time.Hour)}
	if !s.IsExpired(past) {
		t.Error("IsExpired(past) = false, want true")
	}

	// Within the 5-minute skew window: expires in 3 minutes, so the store
	// should still treat it as expired.
	withinSkew := &StoredToken{ExpiresAt: time.Now().Add(3 * time.Minute)}
	if !s.IsExpired(withinSkew) {
		t.Error("IsExpired(within skew) = false, want true")
	}

	future := &StoredToken{ExpiresAt: time.Now().Add(24 * time.Hour)}
	if s.IsExpired(future) {
		t.Error("IsExpired(far future) = true, want false")
	}
}
