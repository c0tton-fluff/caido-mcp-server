package tools

import "fmt"

// maxRawRequestBytes caps the byte length of a raw HTTP request (1MB) accepted
// by the send/forward/workflow/race/convert tools.
const maxRawRequestBytes = 1 << 20

// clampLimit normalizes a user-supplied page limit: v<=0 returns def, v>max
// returns max, otherwise v.
func clampLimit(v, def, max int) int {
	if v <= 0 {
		return def
	}
	if v > max {
		return max
	}
	return v
}

// pageCursor derives (hasMore, nextCursor) from a GraphQL PageInfo. The cursor
// is only surfaced when there is a next page (matching the original per-call
// logic); a nil end cursor yields the empty string.
func pageCursor(hasNext bool, end *string) (bool, string) {
	if !hasNext || end == nil {
		return hasNext, ""
	}
	return hasNext, *end
}

// checkRawSize returns a standardized error when raw exceeds maxRawRequestBytes.
// name identifies the offending input in the message (e.g. "raw",
// "requests[3]", "input").
func checkRawSize(name, raw string) error {
	if len(raw) > maxRawRequestBytes {
		return fmt.Errorf(
			"%s: raw request exceeds max length of 1MB (%d bytes)",
			name, len(raw),
		)
	}
	return nil
}
