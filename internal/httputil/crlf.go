package httputil

import "strings"

// NormalizeCRLF gives a raw request CRLF line endings in its request line
// and headers and guarantees the blank line that ends them. The body after
// that blank line is left byte for byte: rewriting its LFs or appending a
// CRLF after it would change what the server receives and break
// Content-Length.
func NormalizeCRLF(raw string) string {
	// Only interpret literal \r\n escapes when the input has no real CR
	// or LF bytes. This supports CLI usage where users type \r\n
	// literally, while protecting body content that legitimately contains
	// the literal sequence (e.g. JSON values with escaped
	// carriage-return/newline) when the input already has real CRLF.
	if !strings.ContainsRune(raw, '\r') && !strings.ContainsRune(raw, '\n') {
		raw = strings.ReplaceAll(raw, `\r\n`, "\r\n")
	}
	var b strings.Builder
	b.Grow(len(raw) + 4)
	for i := 0; ; {
		j := strings.IndexByte(raw[i:], '\n')
		if j < 0 {
			// No blank line: everything left is header text.
			if line := strings.TrimSuffix(raw[i:], "\r"); line != "" {
				b.WriteString(line)
				b.WriteString("\r\n")
			}
			b.WriteString("\r\n")
			return b.String()
		}
		line := strings.TrimSuffix(raw[i:i+j], "\r")
		i += j + 1
		if line == "" {
			b.WriteString("\r\n")
			b.WriteString(raw[i:])
			return b.String()
		}
		b.WriteString(line)
		b.WriteString("\r\n")
	}
}

func ParseHostHeader(raw string) string {
	for line := range strings.SplitSeq(raw, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), "host:") {
			return strings.TrimSpace(line[5:])
		}
	}
	return ""
}
