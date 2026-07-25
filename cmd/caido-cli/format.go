package main

import (
	"fmt"
	"strings"

	"github.com/c0tton-fluff/caido-mcp-server/internal/httputil"
)

var secHeaders = map[string]bool{
	"content-type":                     true,
	"set-cookie":                       true,
	"location":                         true,
	"www-authenticate":                 true,
	"x-frame-options":                  true,
	"x-content-type-options":           true,
	"content-security-policy":          true,
	"strict-transport-security":        true,
	"access-control-allow-origin":      true,
	"access-control-allow-credentials": true,
	"access-control-allow-methods":     true,
	"server":                           true,
	"x-powered-by":                     true,
	"x-ratelimit-limit":                true,
	"x-ratelimit-remaining":            true,
	"retry-after":                      true,
	"cache-control":                    true,
	"transfer-encoding":                true,
	"content-disposition":              true,
	"x-request-id":                     true,
}

func fmtResp(p *httputil.ParsedMessage, allHeaders bool) string {
	if p == nil {
		return "(no response)"
	}

	status := ""
	if p.FirstLine != "" {
		parts := strings.SplitN(p.FirstLine, " ", 3)
		if len(parts) >= 2 {
			status = parts[1]
		}
	}

	ct := ""
	var hdrs []string
	for _, h := range p.Headers {
		if strings.EqualFold(h.Name, "content-type") {
			ct = strings.SplitN(h.Value, ";", 2)[0]
		} else if allHeaders || secHeaders[strings.ToLower(h.Name)] {
			hdrs = append(hdrs, h.Name+": "+h.Value)
		}
	}

	var lines []string
	lines = append(lines,
		fmt.Sprintf("%s %s %dB", status, ct, p.BodySize),
	)
	lines = append(lines, hdrs...)
	lines = append(lines, "---")
	if p.Body != "" {
		lines = append(lines, p.Body)
	}
	if p.Truncated {
		lines = append(lines,
			fmt.Sprintf("[...truncated, %dB total]", p.BodySize),
		)
	}
	return strings.Join(lines, "\n")
}

func fmtReq(p *httputil.ParsedMessage) string {
	if p == nil {
		return "(no request)"
	}
	var lines []string
	if p.FirstLine != "" {
		lines = append(lines, p.FirstLine)
	}
	for _, h := range p.Headers {
		lines = append(lines, h.Name+": "+h.Value)
	}
	if p.Body != "" {
		lines = append(lines, "")
		lines = append(lines, p.Body)
	}
	return strings.Join(lines, "\n")
}
