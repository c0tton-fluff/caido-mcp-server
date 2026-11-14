package testutil

import (
	"encoding/base64"
	"fmt"
)

// B64 base64-encodes s, the way Caido serializes a Blob.
func B64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func RawHTTPResponse(status int, body string) string {
	raw := fmt.Sprintf(
		"HTTP/1.1 %d OK\r\nContent-Type: text/html\r\nContent-Length: %d\r\n\r\n%s",
		status, len(body), body,
	)
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

func RawHTTPRequest(method, path, host string) string {
	raw := fmt.Sprintf(
		"%s %s HTTP/1.1\r\nHost: %s\r\n\r\n",
		method, path, host,
	)
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

func ListRequestsResponse(ids ...string) map[string]any {
	edges := make([]map[string]any, len(ids))
	for i, id := range ids {
		edges[i] = map[string]any{
			"node": map[string]any{
				"id":     id,
				"method": "GET",
				"host":   "example.com",
				"port":   443,
				"path":   "/api/" + id,
				"query":  "",
				"isTls":  true,
				"response": map[string]any{
					"statusCode": 200,
				},
			},
		}
	}
	return map[string]any{
		"requests": map[string]any{
			"edges": edges,
			"pageInfo": map[string]any{
				"hasNextPage": false,
				"endCursor":   nil,
			},
		},
	}
}

func GetRequestMetadataResponse(id string) map[string]any {
	return map[string]any{
		"request": map[string]any{
			"id":        id,
			"method":    "GET",
			"host":      "example.com",
			"port":      443,
			"path":      "/test",
			"query":     "",
			"isTls":     true,
			"createdAt": int64(1714900000000),
			"response": map[string]any{
				"statusCode":    200,
				"roundtripTime": 42,
			},
		},
	}
}

func GetRequestFullResponse(id string, body string) map[string]any {
	return map[string]any{
		"request": map[string]any{
			"id":        id,
			"method":    "POST",
			"host":      "example.com",
			"port":      443,
			"path":      "/submit",
			"query":     "",
			"isTls":     true,
			"createdAt": int64(1714900000000),
			"raw":       RawHTTPRequest("POST", "/submit", "example.com"),
			"response": map[string]any{
				"statusCode":    200,
				"roundtripTime": 55,
				"raw":           RawHTTPResponse(200, body),
			},
		},
	}
}

// CreateReplaySessionResponse mirrors the Caido 0.57 createReplaySession
// payload: the session is a ReplaySession interface (HTTP variant).
func CreateReplaySessionResponse(sessionID string) map[string]any {
	return map[string]any{
		"createReplaySession": map[string]any{
			"session": map[string]any{
				"__typename":  "ReplaySessionHttp",
				"id":          sessionID,
				"name":        "test-session",
				"activeEntry": nil,
			},
		},
	}
}

// CreateReplaySessionSeededResponse is like CreateReplaySessionResponse
// but with a seeded active entry (as when created with a request source).
func CreateReplaySessionSeededResponse(sessionID, entryID string) map[string]any {
	return map[string]any{
		"createReplaySession": map[string]any{
			"session": map[string]any{
				"__typename": "ReplaySessionHttp",
				"id":         sessionID,
				"name":       "test-session",
				"activeEntry": map[string]any{
					"__typename": "ReplayEntryHttp",
					"id":         entryID,
				},
			},
		},
	}
}

func GetReplaySessionResponse(sessionID, activeEntryID string) map[string]any {
	var activeEntry any
	if activeEntryID != "" {
		activeEntry = map[string]any{
			"__typename": "ReplayEntryHttp",
			"id":         activeEntryID,
		}
	}
	return map[string]any{
		"replaySession": map[string]any{
			"__typename":  "ReplaySessionHttp",
			"id":          sessionID,
			"name":        "test-session",
			"activeEntry": activeEntry,
			"collection":  map[string]any{"id": "col-1", "name": "col"},
			"settings": map[string]any{
				"connectionClose":     false,
				"updateContentLength": true,
			},
			"entries": map[string]any{
				"edges":    []any{},
				"pageInfo": map[string]any{"hasNextPage": false},
			},
		},
	}
}

func StartReplayTaskResponse() map[string]any {
	return map[string]any{
		"startReplayTask": map[string]any{
			"task":  map[string]any{"id": "task-1"},
			"error": nil,
		},
	}
}

// UpdateReplayEntryDraftResponse mirrors the 0.57 updateReplayEntryDraft
// payload.
func UpdateReplayEntryDraftResponse(entryID string) map[string]any {
	return map[string]any{
		"updateReplayEntryDraft": map[string]any{
			"entry": map[string]any{
				"__typename": "ReplayEntryHttp",
				"id":         entryID,
			},
		},
	}
}

func RenameReplaySessionResponse(sessionID, name string) map[string]any {
	return map[string]any{
		"renameReplaySession": map[string]any{
			"session": map[string]any{
				"__typename": "ReplaySessionHttp",
				"id":         sessionID,
				"name":       name,
			},
		},
	}
}

func GetReplayEntryResponse(entryID, requestID string, statusCode int, body string) map[string]any {
	return map[string]any{
		"replayEntry": map[string]any{
			"__typename": "ReplayEntryHttp",
			"id":         entryID,
			"raw":        RawHTTPRequest("GET", "/test", "example.com"),
			"error":      nil,
			"createdAt":  int64(1714900000000),
			"connection": map[string]any{
				"host":  "example.com",
				"port":  443,
				"isTLS": true,
			},
			"settings": map[string]any{
				"placeholders": []any{},
			},
			"request": map[string]any{
				"id":        requestID,
				"method":    "GET",
				"host":      "example.com",
				"port":      443,
				"path":      "/test",
				"query":     "",
				"isTls":     true,
				"raw":       RawHTTPRequest("GET", "/test", "example.com"),
				"createdAt": int64(1714900000000),
				"response": map[string]any{
					"id":            "resp-" + requestID,
					"statusCode":    statusCode,
					"roundtripTime": 100,
					"raw":           RawHTTPResponse(statusCode, body),
					"length":        len(body),
				},
			},
		},
	}
}
