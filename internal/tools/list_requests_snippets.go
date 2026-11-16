package tools

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	caido "github.com/caido-community/sdk-go"
)

const (
	// snippetDefaultContext is how many bytes of context each side of a match
	// carries by default.
	snippetDefaultContext = 40
	// snippetMaxContext is the most context an agent can ask for per side.
	// Exceeding it is clamped, not an error, like limit.
	snippetMaxContext = 200
	// snippetDefaultMatches is how many matches are reported per request.
	snippetDefaultMatches = 3
	// snippetMaxMatches caps matches per request so a broad regex cannot
	// turn one page into a wall of excerpts.
	snippetMaxMatches = 10
	// snippetScanBytes is the most of one part that is searched. Anything
	// beyond it is reported as scanTruncated rather than read.
	snippetScanBytes = 1 << 20
	// snippetMaxMatchBytes caps how much of a very long match lands in the
	// context excerpt. The offset and the part still identify it.
	snippetMaxMatchBytes = 512
	// snippetConcurrency bounds parallel raw fetches for one page.
	snippetConcurrency = 8
	// snippetMaxRegex caps the pattern length before compilation.
	snippetMaxRegex = 1000
)

// snippetParts are the message parts a snippet can search, in the order the
// handler scans them by default.
var snippetParts = []string{
	"responseBody", "requestBody", "responseHeaders", "requestHeaders",
}

// ListRequestsSnippetInput selects a per-row regex scan. The scan annotates
// the rows the httpql filter already selected; it never selects rows by
// itself.
type ListRequestsSnippetInput struct {
	Regex string `json:"regex" jsonschema:"required,RE2 regular expression; linear time, no backtracking"`
	// Context is bytes of excerpt on each side of a match.
	Context int `json:"context,omitempty" jsonschema:"Bytes of context on each side of a match (default 40, max 200)"`
	// MaxPerRequest is how many matches a row reports.
	MaxPerRequest int `json:"maxPerRequest,omitempty" jsonschema:"Matches reported per request (default 3, max 10)"`
	// Parts are the message parts to search.
	Parts []string `json:"parts,omitempty" jsonschema:"Parts to search: responseBody (default), requestBody, responseHeaders, requestHeaders"`
}

// RequestMatch is one regex hit inside a request or response part.
type RequestMatch struct {
	Part   string `json:"part"`
	Offset int    `json:"offset"`
	// ContextOffset is the byte offset of Context within the same part.
	ContextOffset int    `json:"contextOffset"`
	Context       string `json:"context"`
}

// snippetSpec is a validated snippet input.
type snippetSpec struct {
	re      *regexp.Regexp
	context int
	max     int
	parts   []string
}

// newSnippetSpec validates a snippet input. A nil input means no scan.
func newSnippetSpec(in *ListRequestsSnippetInput) (*snippetSpec, error) {
	if in == nil {
		return nil, nil
	}
	if in.Regex == "" {
		return nil, fmt.Errorf("snippet.regex is required")
	}
	if len(in.Regex) > snippetMaxRegex {
		return nil, fmt.Errorf(
			"snippet.regex exceeds max length of %d", snippetMaxRegex,
		)
	}
	re, err := regexp.Compile(in.Regex)
	if err != nil {
		return nil, fmt.Errorf("snippet.regex is not a valid RE2 pattern: %w", err)
	}
	if re.MatchString("") {
		return nil, fmt.Errorf(
			"snippet.regex must not match the empty string",
		)
	}

	contextBytes := in.Context
	if contextBytes <= 0 {
		contextBytes = snippetDefaultContext
	}
	contextBytes = min(contextBytes, snippetMaxContext)

	maxPerRequest := in.MaxPerRequest
	if maxPerRequest <= 0 {
		maxPerRequest = snippetDefaultMatches
	}
	maxPerRequest = min(maxPerRequest, snippetMaxMatches)

	parts := in.Parts
	if len(parts) == 0 {
		parts = []string{"responseBody"}
	}
	seen := make(map[string]bool, len(parts))
	ordered := make([]string, 0, len(parts))
	for _, part := range parts {
		if !slices.Contains(snippetParts, part) {
			return nil, fmt.Errorf(
				"snippet.parts must be one of [%s], got %q",
				strings.Join(snippetParts, " "), part,
			)
		}
		if seen[part] {
			continue
		}
		seen[part] = true
		ordered = append(ordered, part)
	}

	return &snippetSpec{
		re:      re,
		context: contextBytes,
		max:     maxPerRequest,
		parts:   ordered,
	}, nil
}

// annotate scans the given rows and fills their match fields. Raw messages are
// fetched per row with bounded concurrency; a row whose fetch fails carries
// the error instead of matches. Rows are never added or removed.
func (s *snippetSpec) annotate(
	ctx context.Context,
	client *caido.Client,
	rows []RequestSummary,
) {
	sem := make(chan struct{}, snippetConcurrency)
	var wg sync.WaitGroup
	for i := range rows {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			s.scanRow(ctx, client, &rows[i])
		}(i)
	}
	wg.Wait()
}

func (s *snippetSpec) scanRow(
	ctx context.Context,
	client *caido.Client,
	row *RequestSummary,
) {
	resp, err := client.Requests.Get(ctx, row.ID)
	if err != nil {
		row.SnippetError = fmt.Sprintf("failed to read request: %v", err)
		return
	}
	if resp.Request == nil {
		row.SnippetError = "request not found"
		return
	}

	r := resp.Request
	reqHeaders, reqBody := splitRaw(r.Raw)
	parts := map[string][]byte{
		"requestHeaders": reqHeaders,
		"requestBody":    reqBody,
	}
	if r.Response != nil {
		respHeaders, respBody := splitRaw(r.Response.Raw)
		parts["responseHeaders"] = respHeaders
		parts["responseBody"] = respBody
	}

	budget := s.max
	for _, part := range s.parts {
		data := parts[part]
		if len(data) == 0 {
			continue
		}
		if len(data) > snippetScanBytes {
			data = data[:snippetScanBytes]
			row.ScanTruncated = true
		}
		// One extra match tells the caller the list is not complete.
		locs := s.re.FindAllIndex(data, budget+1)
		if len(locs) > budget {
			locs = locs[:budget]
			row.MatchesTruncated = true
		}
		for _, loc := range locs {
			row.Matches = append(row.Matches, s.match(part, data, loc[0], loc[1]))
			budget--
		}
		if budget == 0 {
			break
		}
	}
}

// match builds the excerpt around one hit. Boundaries are moved to rune
// starts so the excerpt is valid UTF-8, and a match longer than
// snippetMaxMatchBytes keeps its head.
func (s *snippetSpec) match(
	part string, data []byte, start, end int,
) RequestMatch {
	from := max(0, start-s.context)
	to := min(len(data), end+s.context)
	if to-from > 2*s.context+snippetMaxMatchBytes {
		to = from + 2*s.context + snippetMaxMatchBytes
	}
	for from < start && !utf8.RuneStart(data[from]) {
		from++
	}
	for to < len(data) && !utf8.RuneStart(data[to]) {
		to--
	}
	return RequestMatch{
		Part:          part,
		Offset:        start,
		ContextOffset: from,
		Context:       string(data[from:to]),
	}
}

// splitRaw decodes a base64 raw message into its header block and body. The
// header block starts at the first line and excludes the blank line.
func splitRaw(raw string) (headers, body []byte) {
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, nil
	}
	headers, body, found := bytes.Cut(decoded, []byte("\r\n\r\n"))
	if !found {
		return headers, nil
	}
	return headers, body
}
