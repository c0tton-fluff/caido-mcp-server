package testutil

import (
	"encoding/base64"
	"fmt"
)

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

func CreateReplaySessionResponse(sessionID string) map[string]any {
	return map[string]any{
		"createReplaySession": map[string]any{
			"session": map[string]any{
				"id": sessionID,
			},
		},
	}
}

func GetReplaySessionResponse(sessionID, activeEntryID string) map[string]any {
	var activeEntry any
	if activeEntryID != "" {
		activeEntry = map[string]any{"id": activeEntryID}
	}
	return map[string]any{
		"replaySession": map[string]any{
			"id":          sessionID,
			"activeEntry": activeEntry,
		},
	}
}

func StartReplayTaskResponse() map[string]any {
	return map[string]any{
		"startReplayTask": map[string]any{
			"error": nil,
		},
	}
}

func GetReplayEntryResponse(entryID, requestID string, statusCode int, body string) map[string]any {
	return map[string]any{
		"replayEntry": map[string]any{
			"id": entryID,
			"request": map[string]any{
				"id":  requestID,
				"raw": RawHTTPRequest("GET", "/test", "example.com"),
				"response": map[string]any{
					"statusCode":    statusCode,
					"roundtripTime": 100,
					"raw":           RawHTTPResponse(statusCode, body),
				},
			},
		},
	}
}
