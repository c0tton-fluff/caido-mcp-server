package httputil

import "strings"

// RedactRawHeaders redacts sensitive header VALUES in a raw HTTP message
// (request or response) so credentials never reach the LLM context. Only header
// lines whose name is in sensitiveHeaders are altered; the request/status line,
// non-sensitive headers, and the body are left byte-for-byte intact. Both CRLF
// and bare-LF line endings are handled, and the first blank line marks the
// header/body boundary after which nothing is touched.
//
// It honors the same opt-out ParseRaw uses: when CAIDO_ALLOW_SENSITIVE_HEADERS
// is truthy the input is returned unchanged (for authorized replay of captured
// authenticated requests).
func RedactRawHeaders(raw string) string {
	if raw == "" || allowSensitiveHeaders() {
		return raw
	}

	var b strings.Builder
	b.Grow(len(raw))

	rest := raw
	firstLine := true
	for rest != "" {
		line, term, tail := splitLine(rest)
		switch {
		case firstLine:
			// Request/status line - never redacted.
			b.WriteString(line)
			b.WriteString(term)
			firstLine = false
		case line == "":
			// Header/body separator: emit it and the untouched body verbatim.
			b.WriteString(term)
			b.WriteString(tail)
			return b.String()
		default:
			b.WriteString(redactHeaderLine(line))
			b.WriteString(term)
		}
		rest = tail
	}
	return b.String()
}

// splitLine splits s at the first line terminator, returning the line content
// (without terminator), the terminator itself ("\r\n", "\n", or "" at EOF), and
// the remainder of s after the terminator.
func splitLine(s string) (line, term, rest string) {
	nl := strings.IndexByte(s, '\n')
	if nl < 0 {
		return s, "", ""
	}
	lineEnd := nl
	if lineEnd > 0 && s[lineEnd-1] == '\r' {
		lineEnd--
	}
	return s[:lineEnd], s[lineEnd : nl+1], s[nl+1:]
}

// redactHeaderLine replaces the value of a sensitive header with [REDACTED],
// leaving non-sensitive header lines byte-for-byte intact.
func redactHeaderLine(line string) string {
	idx := strings.Index(line, ":")
	if idx <= 0 {
		return line
	}
	name := strings.ToLower(strings.TrimSpace(line[:idx]))
	if !sensitiveHeaders[name] {
		return line
	}
	return line[:idx+1] + " [REDACTED]"
}
