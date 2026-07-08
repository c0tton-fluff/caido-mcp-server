package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseTokens(t *testing.T) {
	// A label of exactly 30 chars is NOT < 30, so it fails the short-label
	// heuristic and the whole segment is treated as an auto-labeled token.
	longLabel := strings.Repeat("a", 30)

	tests := []struct {
		name string
		raw  string
		want []tokenDef
	}{
		{
			name: "empty returns nil",
			raw:  "",
			want: nil,
		},
		{
			name: "labeled tokens plus noauth",
			raw:  "owner=eyJ1abc,cross=eyJ2def,noauth",
			want: []tokenDef{
				{Label: "owner", Value: "eyJ1abc"},
				{Label: "cross", Value: "eyJ2def"},
				{Label: "noauth", Value: ""},
			},
		},
		{
			name: "bare token gets auto label",
			raw:  "eyJhbGciOiJIUzI1NiJ9",
			want: []tokenDef{
				{Label: "tok-1", Value: "eyJhbGciOiJIUzI1NiJ9"},
			},
		},
		{
			name: "none is alias for noauth",
			raw:  "none",
			want: []tokenDef{
				{Label: "noauth", Value: ""},
			},
		},
		{
			// len(label) == 30 is not < 30, so the short-label branch is
			// skipped and the entire "label=value" becomes the token value.
			name: "long 30-char label with = falls through to auto label",
			raw:  longLabel + "=secretval",
			want: []tokenDef{
				{Label: "tok-1", Value: longLabel + "=secretval"},
			},
		},
		{
			// A label starting with eyJ fails !HasPrefix(label,"eyJ"), so it
			// also falls through to an auto label with the whole part as value.
			name: "eyJ-prefixed label falls through to auto label",
			raw:  "eyJshort=value",
			want: []tokenDef{
				{Label: "tok-1", Value: "eyJshort=value"},
			},
		},
		{
			name: "multiple auto labels increment index",
			raw:  "eyJaaa,eyJbbb",
			want: []tokenDef{
				{Label: "tok-1", Value: "eyJaaa"},
				{Label: "tok-2", Value: "eyJbbb"},
			},
		},
		{
			name: "blank segments skipped and whitespace trimmed",
			raw:  " owner=tok1 , , noauth ",
			want: []tokenDef{
				{Label: "owner", Value: "tok1"},
				{Label: "noauth", Value: ""},
			},
		},
		{
			// Only labeled/auto tokens advance autoIdx; noauth does not, and a
			// labeled token between two bare tokens must not consume an index.
			name: "labeled and noauth do not advance auto index",
			raw:  "eyJaaa,owner=tok,noauth,eyJbbb",
			want: []tokenDef{
				{Label: "tok-1", Value: "eyJaaa"},
				{Label: "owner", Value: "tok"},
				{Label: "noauth", Value: ""},
				{Label: "tok-2", Value: "eyJbbb"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseTokens(tt.raw)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseTokens(%q) = %#v, want %#v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestApplyToken(t *testing.T) {
	tests := []struct {
		name     string
		start    map[string]string
		token    string
		authMode string
		wantKey  string
		wantVal  string
	}{
		{
			name:     "bearer adds prefix when absent",
			start:    map[string]string{},
			token:    "abc123",
			authMode: "bearer",
			wantKey:  "Authorization",
			wantVal:  "Bearer abc123",
		},
		{
			name:     "bearer keeps existing Bearer prefix",
			start:    map[string]string{},
			token:    "Bearer xyz",
			authMode: "bearer",
			wantKey:  "Authorization",
			wantVal:  "Bearer xyz",
		},
		{
			name:     "bearer keeps existing Basic prefix",
			start:    map[string]string{},
			token:    "Basic dXNlcjpwYXNz",
			authMode: "bearer",
			wantKey:  "Authorization",
			wantVal:  "Basic dXNlcjpwYXNz",
		},
		{
			name:     "empty token is a no-op",
			start:    map[string]string{},
			token:    "",
			authMode: "bearer",
			wantKey:  "Authorization",
			wantVal:  "",
		},
		{
			name:     "cookie sets header when absent",
			start:    map[string]string{},
			token:    "tok",
			authMode: "cookie:session",
			wantKey:  "Cookie",
			wantVal:  "session=tok",
		},
		{
			name:     "cookie appends to existing non-empty Cookie",
			start:    map[string]string{"Cookie": "foo=bar"},
			token:    "tok",
			authMode: "cookie:session",
			wantKey:  "Cookie",
			wantVal:  "foo=bar; session=tok",
		},
		{
			name:     "cookie replaces existing empty Cookie",
			start:    map[string]string{"Cookie": ""},
			token:    "tok",
			authMode: "cookie:session",
			wantKey:  "Cookie",
			wantVal:  "session=tok",
		},
		{
			name:     "header sets named header",
			start:    map[string]string{},
			token:    "tok",
			authMode: "header:X-Api-Key",
			wantKey:  "X-Api-Key",
			wantVal:  "tok",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := tt.start
			applyToken(h, tt.token, tt.authMode)
			if got := h[tt.wantKey]; got != tt.wantVal {
				t.Errorf("applyToken() header[%q] = %q, want %q",
					tt.wantKey, got, tt.wantVal)
			}
		})
	}
}
