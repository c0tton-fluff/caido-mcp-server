package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/c0tton-fluff/caido-mcp-server/internal/httputil"
	"github.com/c0tton-fluff/caido-mcp-server/internal/replay"
	caido "github.com/caido-community/sdk-go"
	gen "github.com/caido-community/sdk-go/graphql"
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

	var prevEntryID string
	sessResp, err := client.Replay.GetSession(ctx, sessionID)
	if err == nil && sessResp.ReplaySession != nil &&
		sessResp.ReplaySession.ActiveEntry != nil {
		prevEntryID = sessResp.ReplaySession.ActiveEntry.Id
	}

	rawB64 := base64.StdEncoding.EncodeToString([]byte(raw))
	taskInput := &gen.StartReplayTaskInput{
		Connection: gen.ConnectionInfoInput{
			Host:  host,
			Port:  port,
			IsTLS: useTLS,
		},
		Raw: rawB64,
		Settings: gen.ReplayEntrySettingsInput{
			Placeholders:        []gen.ReplayPlaceholderInput{},
			UpdateContentLength: true,
			ConnectionClose:     false,
		},
	}

	taskResp, err := client.Replay.SendRequest(
		ctx, sessionID, taskInput,
	)
	hasError := taskResp != nil &&
		taskResp.StartReplayTask.GetError() != nil
	if err != nil || hasError {
		isTaskBusy := false
		if err != nil {
			isTaskBusy = strings.Contains(
				err.Error(), "TaskInProgressUserError",
			)
		} else {
			isTaskBusy = true
		}

		if isTaskBusy {
			newResp, createErr := client.Replay.CreateSession(
				ctx, &gen.CreateReplaySessionInput{},
			)
			if createErr != nil {
				return "", fmt.Errorf(
					"fallback session: %w", createErr,
				)
			}
			sessionID = newResp.CreateReplaySession.Session.Id
			replay.ResetDefaultSession(sessionID)
			prevEntryID = ""
			_, err = client.Replay.SendRequest(
				ctx, sessionID, taskInput,
			)
			if err != nil {
				return "", fmt.Errorf("send retry: %w", err)
			}
		} else if err != nil {
			return "", fmt.Errorf("send: %w", err)
		}
	}

	entry, err := replay.PollForEntry(ctx, client, sessionID, prevEntryID)
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
