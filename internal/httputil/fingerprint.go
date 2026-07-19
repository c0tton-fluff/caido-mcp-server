package httputil

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

type ContentKind string

const (
	KindJSON   ContentKind = "json"
	KindHTML   ContentKind = "html"
	KindXML    ContentKind = "xml"
	KindText   ContentKind = "text"
	KindBinary ContentKind = "binary"
	KindEmpty  ContentKind = "empty"
)

type Fingerprint struct {
	Kind        ContentKind `json:"kind"`
	ContentType string      `json:"contentType,omitempty"`
	BodySize    int         `json:"bodySize"`
	// StatusCode, Title, RedirectTarget, SetCookies, and WordCount are
	// response-only fields populated by PopulateResponseDetails, not by
	// FingerprintFromHeaders/FingerprintFromBody (those run inside
	// ParseRaw before the status line and full header/body set are
	// available together). Zero-valued when the caller never invokes
	// PopulateResponseDetails, e.g. for request fingerprints.
	StatusCode     int      `json:"statusCode,omitempty"`
	Title          string   `json:"title,omitempty"`
	RedirectTarget string   `json:"redirectTarget,omitempty"`
	SetCookies     []string `json:"setCookies,omitempty"`
	WordCount      int      `json:"wordCount,omitempty"`
	// NotableHeaders surfaces response headers that are NOT part of the
	// common standard/transport/caching/security set (see commonHeaders).
	// This is where custom application signal hides -- framework banners
	// (Server, X-Powered-By), debug/trace headers, internal hostnames, and
	// CTF-style flag headers (X-Flag) -- which the other fingerprint fields
	// never capture. Populated by PopulateResponseDetails from the same
	// (already sensitive-redacted) header list the caller parsed.
	NotableHeaders []Header `json:"notableHeaders,omitempty"`
}

func FingerprintFromHeaders(headers []Header, bodySize int) Fingerprint {
	fp := Fingerprint{BodySize: bodySize}

	if bodySize == 0 {
		fp.Kind = KindEmpty
		return fp
	}

	ct := headerValue(headers, "content-type")
	fp.ContentType = ct

	lower := strings.ToLower(ct)
	switch {
	case strings.Contains(lower, "json"):
		fp.Kind = KindJSON
	case strings.Contains(lower, "html"):
		fp.Kind = KindHTML
	case strings.Contains(lower, "xml"):
		fp.Kind = KindXML
	case strings.Contains(lower, "text"):
		fp.Kind = KindText
	case strings.Contains(lower, "javascript"):
		fp.Kind = KindText
	case strings.Contains(lower, "image"),
		strings.Contains(lower, "audio"),
		strings.Contains(lower, "video"),
		strings.Contains(lower, "octet-stream"),
		strings.Contains(lower, "font"),
		strings.Contains(lower, "woff"),
		strings.Contains(lower, "pdf"):
		fp.Kind = KindBinary
	default:
		fp.Kind = KindText
	}

	return fp
}

func FingerprintFromBody(body []byte) Fingerprint {
	fp := Fingerprint{BodySize: len(body)}

	if len(body) == 0 {
		fp.Kind = KindEmpty
		return fp
	}

	if !utf8.Valid(body) {
		fp.Kind = KindBinary
		return fp
	}

	trimmed := strings.TrimSpace(string(body[:min(len(body), 256)]))
	switch {
	case strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "["):
		fp.Kind = KindJSON
	case strings.HasPrefix(strings.ToLower(trimmed), "<!doctype") ||
		strings.HasPrefix(strings.ToLower(trimmed), "<html"):
		fp.Kind = KindHTML
	case strings.HasPrefix(trimmed, "<?xml") || strings.HasPrefix(trimmed, "<"):
		fp.Kind = KindXML
	default:
		fp.Kind = KindText
	}

	return fp
}

func AdaptiveBodyLimit(fp Fingerprint, requestedLimit int) int {
	if requestedLimit > 0 {
		return requestedLimit
	}
	switch fp.Kind {
	case KindJSON:
		return 4000
	case KindHTML:
		return 3000
	case KindXML:
		return 3000
	case KindBinary:
		return 200
	case KindEmpty:
		return 0
	default:
		return DefaultBodyLimit
	}
}

func headerValue(headers []Header, name string) string {
	lower := strings.ToLower(name)
	for _, h := range headers {
		if strings.ToLower(h.Name) == lower {
			return h.Value
		}
	}
	return ""
}

// headerValues returns every value of headers matching name
// case-insensitively, in encounter order. Unlike headerValue, this
// captures repeated headers such as Set-Cookie.
func headerValues(headers []Header, name string) []string {
	lower := strings.ToLower(name)
	var values []string
	for _, h := range headers {
		if strings.ToLower(h.Name) == lower {
			values = append(values, h.Value)
		}
	}
	return values
}

var titleTagRe = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)

// ExtractTitle returns the trimmed text of the first <title> element found
// in body, or "" when none is present (including for non-HTML bodies).
// Matching is case-insensitive; markup nested inside the title is not
// stripped.
func ExtractTitle(body []byte) string {
	m := titleTagRe.FindSubmatch(body)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(string(m[1]))
}

// WordCount returns the whitespace-split token count of body.
func WordCount(body []byte) int {
	return len(strings.Fields(string(body)))
}

// commonHeaders are ordinary response headers that carry no per-app signal:
// content negotiation, transport, caching, CORS, and standard security
// headers. NotableHeaders excludes these so what remains is the unusual
// stuff worth a human's attention. location and set-cookie are excluded
// because they already have dedicated fingerprint fields (RedirectTarget,
// SetCookies).
var commonHeaders = map[string]bool{
	"location": true, "set-cookie": true,
	"content-type": true, "content-length": true, "content-encoding": true,
	"content-language": true, "content-range": true, "content-disposition": true,
	"date": true, "connection": true, "keep-alive": true, "transfer-encoding": true,
	"vary": true, "accept-ranges": true, "age": true, "expires": true,
	"cache-control": true, "pragma": true, "etag": true, "last-modified": true,
	"access-control-allow-origin": true, "access-control-allow-credentials": true,
	"access-control-allow-methods": true, "access-control-allow-headers": true,
	"access-control-expose-headers": true, "access-control-max-age": true,
	"strict-transport-security": true, "x-content-type-options": true,
	"x-frame-options": true, "x-xss-protection": true, "content-security-policy": true,
	"referrer-policy": true, "permissions-policy": true, "cross-origin-opener-policy": true,
	"cross-origin-resource-policy": true, "cross-origin-embedder-policy": true,
	"report-to": true, "nel": true, "alt-svc": true, "upgrade": true,
}

// maxNotableHeaders caps NotableHeaders so a pathological response cannot
// bloat the tool output; the dropped count is not reported since the intent
// is a summary, not an exhaustive dump (the full list stays in Headers).
const maxNotableHeaders = 25

// NotableHeaders returns the subset of headers whose names are not in
// commonHeaders, preserving encounter order and stopping at
// maxNotableHeaders. Always returns nil (not an empty slice) when nothing is
// notable, so it stays omitempty in JSON output.
func NotableHeaders(headers []Header) []Header {
	var notable []Header
	for _, h := range headers {
		if commonHeaders[strings.ToLower(h.Name)] {
			continue
		}
		notable = append(notable, h)
		if len(notable) == maxNotableHeaders {
			break
		}
	}
	return notable
}

// SetCookieNames extracts cookie NAMES (never values) from a list of raw
// Set-Cookie header values such as "session=abc123; Path=/". A value that
// has already been redacted by ParseRaw's sensitive-header handling (i.e.
// "[REDACTED]") yields no name, since the name is embedded in the same
// value that was redacted; set CAIDO_ALLOW_SENSITIVE_HEADERS to recover
// it. Always returns a non-nil (possibly empty) slice.
func SetCookieNames(values []string) []string {
	names := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" || v == "[REDACTED]" {
			continue
		}
		if idx := strings.IndexByte(v, '='); idx > 0 {
			names = append(names, strings.TrimSpace(v[:idx]))
		}
	}
	return names
}

// PopulateResponseDetails fills in the response-only Fingerprint fields
// (StatusCode, Title, RedirectTarget, SetCookies, WordCount) that
// FingerprintFromHeaders/FingerprintFromBody cannot set on their own:
// those two run inside ParseRaw before the response status line and the
// full header list are available together with the decoded body. Callers
// that already have all three (send_request, batch_send) call this once,
// after building the base Fingerprint via ParseBase64/ParseRaw, passing
// the same headers/body they parsed.
//
// ASSUMPTION: Title and WordCount reflect exactly the body slice passed
// in. If the caller's body was truncated upstream (e.g. a user-requested
// bodyLimit), so are these derived values -- callers wanting
// fully-accurate results should pass an untruncated body where available.
// No-op when fp is nil.
func PopulateResponseDetails(fp *Fingerprint, statusCode int, headers []Header, body []byte) {
	if fp == nil {
		return
	}
	fp.StatusCode = statusCode
	fp.RedirectTarget = headerValue(headers, "location")
	fp.SetCookies = SetCookieNames(headerValues(headers, "set-cookie"))
	fp.NotableHeaders = NotableHeaders(headers)
	if fp.Kind == KindHTML {
		fp.Title = ExtractTitle(body)
	}
	if len(body) > 0 {
		fp.WordCount = WordCount(body)
	}
}
