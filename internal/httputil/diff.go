package httputil

import (
	"fmt"
	"strings"
	"sync"
)

type ResponseDigest struct {
	StatusCode int
	BodyHash   uint64
	BodySize   int
	BodySnip   string
}

type DiffResult struct {
	Same         bool   `json:"same"`
	StatusChange string `json:"statusChange,omitempty"`
	SizeChange   string `json:"sizeChange,omitempty"`
	Summary      string `json:"summary,omitempty"`
}

type ResponseCache struct {
	mu      sync.Mutex
	entries map[string]ResponseDigest
}

var globalCache = &ResponseCache{
	entries: make(map[string]ResponseDigest),
}

func GlobalResponseCache() *ResponseCache {
	return globalCache
}

func (c *ResponseCache) GetAndSet(sessionID string, current ResponseDigest) *DiffResult {
	c.mu.Lock()
	prev, exists := c.entries[sessionID]
	c.entries[sessionID] = current
	c.mu.Unlock()

	if !exists {
		return nil
	}

	if prev.BodyHash == current.BodyHash && prev.StatusCode == current.StatusCode &&
		prev.BodySize == current.BodySize {
		return &DiffResult{
			Same:    true,
			Summary: "identical to previous response",
		}
	}

	diff := &DiffResult{}

	if prev.StatusCode != current.StatusCode {
		diff.StatusChange = fmt.Sprintf("%d -> %d", prev.StatusCode, current.StatusCode)
	}

	if prev.BodySize != current.BodySize {
		delta := current.BodySize - prev.BodySize
		sign := "+"
		if delta < 0 {
			sign = ""
		}
		diff.SizeChange = fmt.Sprintf("%s%d bytes", sign, delta)
	}

	if diff.StatusChange != "" || diff.SizeChange != "" {
		var parts []string
		if diff.StatusChange != "" {
			parts = append(parts, "status "+diff.StatusChange)
		}
		if diff.SizeChange != "" {
			parts = append(parts, "body "+diff.SizeChange)
		}
		diff.Summary = "changed: " + strings.Join(parts, ", ")
	}

	return diff
}

func (c *ResponseCache) Clear(sessionID string) {
	c.mu.Lock()
	delete(c.entries, sessionID)
	c.mu.Unlock()
}

func HashBody(body []byte) uint64 {
	var h uint64 = 14695981039346656037
	for _, b := range body {
		h ^= uint64(b)
		h *= 1099511628211
	}
	return h
}
