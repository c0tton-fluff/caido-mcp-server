package main

import (
	"context"
	"fmt"

	"github.com/c0tton-fluff/caido-mcp-server/internal/httputil"
	"github.com/c0tton-fluff/caido-mcp-server/internal/replay"
	caido "github.com/caido-community/sdk-go"
)

// sendReplay sends a CRLF-normalized raw HTTP request via the Replay API
// and returns the terse-formatted response string.
func sendReplay(
	ctx context.Context,
	client *caido.Client,
	raw, host string,
	port int, useTLS bool,
	bodyLimit int, allHeaders bool,
) (string, error) {
	sessionID, err := replay.GetOrCreateSession(ctx, client, "")
	if err != nil {
		return "", err
	}

	conn := caido.ReplayConnection{Host: host, Port: port, IsTLS: useTLS}
	sendRes, err := replay.Send(ctx, client, sessionID, raw, conn, true)
	if err != nil {
		return "", fmt.Errorf("send: %w", err)
	}

	entry, err := replay.PollForEntry(
		ctx, client, sendRes.SessionID, sendRes.PreviousEntryID,
	)
	if err != nil {
		return "", err
	}

	if entry.Request == nil || entry.Request.Response == nil {
		return "", fmt.Errorf("no response received")
	}

	resp := httputil.ParseBase64(
		entry.Request.Response.Raw, true, true, 0, bodyLimit,
	)
	return fmtResp(resp, allHeaders) + "\n", nil
}
