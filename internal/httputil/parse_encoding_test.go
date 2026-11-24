package httputil

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

// pngBody is the 8-byte PNG signature followed by every byte value twice, so
// the result is guaranteed not to be valid UTF-8. The exact size does not
// matter; tests slice it at rune-hostile offsets on purpose.
func pngBody() []byte {
	body := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}
	for range 2 {
		for b := range 256 {
			body = append(body, byte(b))
		}
	}
	return body
}

func rawMessage(contentType string, body []byte) []byte {
	var b bytes.Buffer
	b.WriteString("HTTP/1.1 200 OK\r\n")
	if contentType != "" {
		b.WriteString("Content-Type: " + contentType + "\r\n")
	}
	b.WriteString("\r\n")
	b.Write(body)
	return b.Bytes()
}

// TestParseRawWith_AutoMatchesLegacyTextOutput asserts that auto mode leaves
// UTF-8 bodies exactly as the text-only parser returned them: same fields,
// same JSON bytes, no envelope.
func TestParseRawWith_AutoMatchesLegacyTextOutput(t *testing.T) {
	bodies := []string{
		"<html><body>hello</body></html>",
		`{"user":"josé","note":"naïve café"}`,
		"line one\nline two\n",
		strings.Repeat("é", 400),
	}
	offsets := [][2]int{{0, 0}, {0, 4}, {4, 0}}

	for _, body := range bodies {
		raw := rawMessage("text/plain; charset=utf-8", []byte(body))
		for _, off := range offsets {
			legacy := ParseRaw(raw, true, true, off[0], off[1])
			auto := ParseRawWith(
				raw, true, true, off[0], off[1], EncodingAuto,
			)

			legacyJSON, err := json.Marshal(legacy)
			if err != nil {
				t.Fatal(err)
			}
			autoJSON, err := json.Marshal(auto)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(legacyJSON, autoJSON) {
				t.Fatalf(
					"auto output changed for %q offset=%d limit=%d:\nlegacy %s\nauto   %s",
					body, off[0], off[1], legacyJSON, autoJSON,
				)
			}
		}
	}
}

// TestParseRawWith_AutoBase64ForBinary asserts auto mode returns a binary body
// as base64 that decodes back to the exact selected bytes.
func TestParseRawWith_AutoBase64ForPNG(t *testing.T) {
	body := pngBody()
	raw := rawMessage("image/png", body)

	result := ParseRawWith(raw, true, true, 0, 0, EncodingAuto)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Body != "" {
		t.Errorf("binary body rendered as text: %q", result.Body)
	}
	if result.BodyEncoding != "base64" {
		t.Errorf("bodyEncoding = %q, want base64", result.BodyEncoding)
	}
	if result.BodySize != len(body) {
		t.Errorf("bodySize = %d, want %d", result.BodySize, len(body))
	}
	if result.BodyOffset == nil || *result.BodyOffset != 0 {
		t.Errorf("bodyOffset = %v, want 0", result.BodyOffset)
	}
	decoded, err := base64.StdEncoding.DecodeString(result.BodyBase64)
	if err != nil {
		t.Fatalf("bodyBase64 does not decode: %v", err)
	}
	if !bytes.Equal(decoded, body) {
		t.Fatal("bodyBase64 does not round-trip to the body bytes")
	}
}

// TestParseBase64With_PNGRoundTripPages pages a binary body by bytes and
// reassembles it, which is how an agent reads an 8 KB image in two calls.
func TestParseBase64With_PNGRoundTripPages(t *testing.T) {
	body := pngBody()
	raw := rawMessage("image/png", body)
	encoded := base64.StdEncoding.EncodeToString(raw)

	var got []byte
	for offset := 0; offset < len(body); offset += 4096 {
		result := ParseBase64With(
			encoded, true, true, offset, 4096, EncodingAuto,
		)
		if result == nil || result.BodyEncoding != "base64" {
			t.Fatalf("page at %d: no base64 body: %+v", offset, result)
		}
		page, err := base64.StdEncoding.DecodeString(result.BodyBase64)
		if err != nil {
			t.Fatalf("page at %d: %v", offset, err)
		}
		got = append(got, page...)
		if result.BodyOffset == nil || *result.BodyOffset != offset {
			t.Errorf(
				"page at %d: bodyOffset = %v", offset, result.BodyOffset,
			)
		}
		if result.BodySize != len(body) {
			t.Errorf(
				"page at %d: bodySize = %d, want %d",
				offset, result.BodySize, len(body),
			)
		}
	}
	if !bytes.Equal(got, body) {
		t.Fatal("paged base64 bytes do not match the body")
	}
}

// TestParseRawWith_TextKeepsCutRuneWhole asserts a byte limit that lands
// inside a rune returns the whole rune with the page it starts in, instead of
// half a rune or a silently shortened page.
func TestParseRawWith_TextKeepsCutRuneWhole(t *testing.T) {
	// "aé" is three bytes; a limit of two cuts é in half. Auto must keep
	// this text, not fall back to base64 because of the cut.
	cases := []struct {
		body      string
		truncated bool
	}{
		{"aé", false}, // the completed rune reaches the end of the body
		{"aéb", true}, // more follows
	}
	for _, tc := range cases {
		raw := rawMessage("text/plain", []byte(tc.body))
		for _, encoding := range []Encoding{EncodingText, EncodingAuto} {
			result := ParseRawWith(raw, false, true, 0, 2, encoding)
			if result == nil {
				t.Fatalf("%s: expected non-nil result", encoding)
			}
			if result.Body != "aé" {
				t.Errorf("%s %q: body = %q, want %q", encoding, tc.body, result.Body, "aé")
			}
			if result.BodyEncoding != "" {
				t.Errorf("%s %q: body became base64", encoding, tc.body)
			}
			if result.Truncated != tc.truncated {
				t.Errorf("%s %q: truncated = %v, want %v", encoding, tc.body, result.Truncated, tc.truncated)
			}
			if result.BodySize != len(tc.body) {
				t.Errorf("%s %q: bodySize = %d, want %d", encoding, tc.body, result.BodySize, len(tc.body))
			}
		}
	}
}

// TestParseRawWith_TextPagesAreLossless pages a multi-byte text body the way
// an agent does, bodyOffset += bodyLimit, at every small page size. Every
// rune must come back exactly once: a rune cut by a page boundary used to
// vanish from both pages.
func TestParseRawWith_TextPagesAreLossless(t *testing.T) {
	body := "naïve café – 日本語 🙂 end"
	raw := rawMessage("application/json", []byte(body))
	for _, encoding := range []Encoding{EncodingText, EncodingAuto} {
		for size := 1; size <= 9; size++ {
			var joined string
			for offset := 0; offset < len(body); offset += size {
				page := ParseRawWith(raw, false, true, offset, size, encoding)
				if page.BodyEncoding != "" {
					t.Fatalf("%s size %d offset %d: text page became base64", encoding, size, offset)
				}
				joined += page.Body
			}
			if joined != body {
				t.Errorf("%s size %d: pages join to %q, want %q", encoding, size, joined, body)
			}
		}
	}
}

// TestParseRawWith_StrayByteAtCutIsNotDropped asserts a page that ends on a
// byte which is not part of any rune is returned as bytes. Treating that byte
// as a cut rune and dropping it would lose it without a trace.
func TestParseRawWith_StrayByteAtCutIsNotDropped(t *testing.T) {
	body := append([]byte("aaaaaaaaa"), 0xFF, 0xFE, 0x00, 0x01)
	raw := rawMessage("application/octet-stream", body)
	result := ParseRawWith(raw, false, true, 0, 10, EncodingAuto)
	if result.BodyEncoding != "base64" {
		t.Fatalf("encoding = %q, body = %q; want base64", result.BodyEncoding, result.Body)
	}
	page, err := base64.StdEncoding.DecodeString(result.BodyBase64)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(page, body[:10]) {
		t.Errorf("page = %x, want %x", page, body[:10])
	}
	if !result.Truncated {
		t.Error("expected truncated")
	}
}

// TestParseRawWith_Base64OffsetIsReported asserts the envelope names the byte
// offset of the slice it carries.
func TestParseRawWith_Base64OffsetIsReported(t *testing.T) {
	body := pngBody()
	result := ParseRawWith(rawMessage("image/png", body), false, true, 200, 16, EncodingAuto)
	if result.BodyOffset == nil || *result.BodyOffset != 200 {
		t.Fatalf("bodyOffset = %v, want 200", result.BodyOffset)
	}
	page, _ := base64.StdEncoding.DecodeString(result.BodyBase64)
	if !bytes.Equal(page, body[200:216]) {
		t.Errorf("page = %x, want %x", page, body[200:216])
	}
}

// TestParseRawWith_TextOffsetMidRune asserts an offset inside a rune does not
// start the text with continuation bytes.
func TestParseRawWith_TextOffsetMidRune(t *testing.T) {
	raw := rawMessage("text/plain", []byte("éé"))
	result := ParseRawWith(raw, false, true, 1, 0, EncodingText)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Body != "é" {
		t.Errorf("body = %q, want one %q", result.Body, "é")
	}
}

// TestParseRawWith_ForcedTextKeepsRawBytes asserts forcing text keeps the old
// behaviour for callers that explicitly ask for it, even on binary bytes.
func TestParseRawWith_ForcedTextKeepsRawBytes(t *testing.T) {
	body := pngBody()
	raw := rawMessage("image/png", body)
	result := ParseRawWith(raw, false, true, 0, 32, EncodingText)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.BodyEncoding != "" || result.BodyBase64 != "" {
		t.Errorf("forced text returned an envelope: %+v", result)
	}
	// The PNG signature starts with 0x89, a UTF-8 continuation byte. It is
	// not the tail of a cut rune, so text mode keeps all 32 bytes, as every
	// release before bodyEncoding did.
	if result.Body != string(body[:32]) {
		t.Errorf("body = %x, want %x", result.Body, body[:32])
	}
}

// TestParseRawWith_ForcedBase64OnText asserts forcing base64 works on a text
// body and that the asset is byte-exact.
func TestParseRawWith_ForcedBase64OnText(t *testing.T) {
	body := []byte(`{"ok":true}`)
	raw := rawMessage("application/json", body)
	result := ParseRawWith(raw, false, true, 0, 0, EncodingBase64)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.BodyEncoding != "base64" {
		t.Fatalf("bodyEncoding = %q", result.BodyEncoding)
	}
	decoded, err := base64.StdEncoding.DecodeString(result.BodyBase64)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded, body) {
		t.Fatalf("decoded %q, want %q", decoded, body)
	}
}

// TestParseRawWith_MediaTypeBinaryRequiresNUL asserts a binary content type is
// only a second signal: ASCII under octet-stream stays text, NULs under the
// same type become base64.
func TestParseRawWith_MediaTypeBinaryRequiresNUL(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        []byte
		wantBase64  bool
	}{
		{"ascii under octet-stream stays text", "application/octet-stream", []byte("plain ascii"), false},
		{"nul under octet-stream", "application/octet-stream", []byte("bin\x00ary"), true},
		{"nul under text/plain stays text", "text/plain", []byte("bin\x00ary"), false},
		{"svg stays text", "image/svg+xml", []byte("<svg/>\x00"), false},
		{"json suffix stays text", "application/problem+json", []byte("{}\x00"), false},
		{"utf8 under text/html", "text/html", []byte("café"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := rawMessage(tt.contentType, tt.body)
			result := ParseRawWith(raw, false, true, 0, 0, EncodingAuto)
			if result == nil {
				t.Fatal("expected non-nil result")
			}
			gotBase64 := result.BodyEncoding == "base64"
			if gotBase64 != tt.wantBase64 {
				t.Fatalf(
					"base64 = %v, want %v (body=%q, encoding=%q)",
					gotBase64, tt.wantBase64, result.Body, result.BodyEncoding,
				)
			}
		})
	}
}

func TestParseEncoding(t *testing.T) {
	tests := []struct {
		in      string
		want    Encoding
		wantErr bool
	}{
		{"", EncodingAuto, false},
		{"auto", EncodingAuto, false},
		{"TEXT", EncodingText, false},
		{" base64 ", EncodingBase64, false},
		{"hex", "", true},
	}
	for _, tt := range tests {
		got, err := ParseEncoding(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ParseEncoding(%q) accepted an unknown value", tt.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseEncoding(%q): %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseEncoding(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
