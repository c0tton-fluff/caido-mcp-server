package replay

import (
	"context"
	"fmt"
	"strings"

	gql "github.com/Khan/genqlient/graphql"
	caido "github.com/caido-community/sdk-go"
)

// WebSocket replay is a different shape from HTTP replay:
//
//   - The entry carries the handshake as its http part (raw request plus the
//     101 response), and the messages as a ReplayEntryWs.
//   - A pending outgoing message lives in the entry's draft, which is a
//     separate object (ReplayEntryWsDraft) that only exists once something
//     has been written into it.
//   - Sending is task based: startReplayTask opens the connection and creates
//     a fresh entry, then sendReplayTaskMessageDraft (or sendReplayTaskMessage)
//     writes frames on it. Every frame, sent or received, is recorded in that
//     entry's messages list.
//
// sdk-go only generates the HTTP half of the replay API, so the operations
// below are hand-written, in the same tolerant plain-struct style as list.go.

const wsEntryQuery = `query GetReplayEntryWs($id: ID!) {
  replayEntry(id: $id, sessionKind: WS) {
    __typename
    id
    error
    createdAt
    ... on ReplayEntryWs {
      http {
        raw
        connection { host port isTLS }
        request {
          response { statusCode roundtripTime raw }
        }
      }
      draft { raw direction format editorState }
      messages { id head { raw direction format length createdAt } }
      stream { id }
    }
  }
}`

const setWsDraftMutation = `mutation SetReplayEntryWsDraft($id: ID!, $input: UpdateReplayEntryDraftInput!) {
  updateReplayEntryDraft(id: $id, input: $input) {
    entry {
      __typename
      ... on ReplayEntryWs {
        draft { raw direction format }
      }
    }
  }
}`

const clearDraftMutation = `mutation ClearReplayWsDraft($id: ID!, $kind: ReplaySessionKind!) {
  clearReplayEntryDraft(id: $id, kind: $kind) {
    entry { __typename id }
  }
}`

// Frame directions accepted by Caido.
const (
	DirectionClient = "CLIENT"
	DirectionServer = "SERVER"
)

// WsDraft is the pending message of a WebSocket replay entry. Raw is a base64
// Blob as the API serializes it.
type WsDraft struct {
	Raw         string
	Direction   string
	Format      string
	EditorState string
}

// WsMessage is one recorded WebSocket frame of a replay entry. Raw is base64.
type WsMessage struct {
	ID        string
	Raw       string
	Direction string
	Format    string
	Length    int
	CreatedAt int64
}

// WsHandshake is the HTTP upgrade the entry replays.
type WsHandshake struct {
	RequestRaw  string
	Host        string
	Port        int
	IsTLS       bool
	StatusCode  int
	RoundtripMs int
	ResponseRaw string
}

// WsEntry is a WebSocket replay entry: handshake, pending draft and every
// frame recorded on the entry so far.
type WsEntry struct {
	ID           string
	Typename     string
	Error        string
	CreatedAt    int64
	StreamID     string
	Handshake    WsHandshake
	Draft        *WsDraft
	Messages     []WsMessage
	IsWebSocket  bool
	EntryMissing bool
}

// WsDraftInput is the value written into a replay entry's draft.
type WsDraftInput struct {
	Raw             string // base64 Blob payload
	Direction       string // CLIENT or SERVER
	Format          string // TEXT, BINARY, PING, PONG or CLOSE
	ServerTimeoutMs int
	EditorState     string // base64 Blob; empty reuses the current draft's state
}

type wsEntryResponse struct {
	ReplayEntry *struct {
		Typename  string  `json:"__typename"`
		ID        string  `json:"id"`
		Error     *string `json:"error"`
		CreatedAt int64   `json:"createdAt"`
		Http      *struct {
			Raw        string `json:"raw"`
			Connection struct {
				Host  string `json:"host"`
				Port  int    `json:"port"`
				IsTLS bool   `json:"isTLS"`
			} `json:"connection"`
			Request *struct {
				Response *struct {
					StatusCode int    `json:"statusCode"`
					Roundtrip  int    `json:"roundtripTime"`
					Raw        string `json:"raw"`
				} `json:"response"`
			} `json:"request"`
		} `json:"http"`
		Draft *struct {
			Raw         string `json:"raw"`
			Direction   string `json:"direction"`
			Format      string `json:"format"`
			EditorState string `json:"editorState"`
		} `json:"draft"`
		Messages []struct {
			ID   string `json:"id"`
			Head struct {
				Raw       string `json:"raw"`
				Direction string `json:"direction"`
				Format    string `json:"format"`
				Length    int    `json:"length"`
				CreatedAt int64  `json:"createdAt"`
			} `json:"head"`
		} `json:"messages"`
		Stream *struct {
			ID string `json:"id"`
		} `json:"stream"`
	} `json:"replayEntry"`
}

// GetWsEntry reads a WebSocket replay entry, including its draft and messages.
// An unknown id yields an entry with EntryMissing set and no error: Caido
// answers a null replayEntry rather than an error for a bad id.
func GetWsEntry(
	ctx context.Context, client *caido.Client, id string,
) (*WsEntry, error) {
	var data wsEntryResponse
	if err := client.GraphQL.MakeRequest(ctx, &gql.Request{
		OpName:    "GetReplayEntryWs",
		Query:     wsEntryQuery,
		Variables: map[string]any{"id": id},
	}, &gql.Response{Data: &data}); err != nil {
		return nil, fmt.Errorf("get ws replay entry %s: %w", id, err)
	}
	node := data.ReplayEntry
	if node == nil {
		return &WsEntry{ID: id, EntryMissing: true}, nil
	}

	out := &WsEntry{
		ID:          node.ID,
		Typename:    node.Typename,
		CreatedAt:   node.CreatedAt,
		IsWebSocket: node.Typename == "ReplayEntryWs",
	}
	if node.Error != nil {
		out.Error = *node.Error
	}
	if node.Http != nil {
		out.Handshake = WsHandshake{
			RequestRaw: node.Http.Raw,
			Host:       node.Http.Connection.Host,
			Port:       node.Http.Connection.Port,
			IsTLS:      node.Http.Connection.IsTLS,
		}
		if node.Http.Request != nil && node.Http.Request.Response != nil {
			out.Handshake.StatusCode = node.Http.Request.Response.StatusCode
			out.Handshake.RoundtripMs = node.Http.Request.Response.Roundtrip
			out.Handshake.ResponseRaw = node.Http.Request.Response.Raw
		}
	}
	if node.Draft != nil {
		out.Draft = &WsDraft{
			Raw:         node.Draft.Raw,
			Direction:   node.Draft.Direction,
			Format:      node.Draft.Format,
			EditorState: node.Draft.EditorState,
		}
	}
	if node.Stream != nil {
		out.StreamID = node.Stream.ID
	}
	for _, m := range node.Messages {
		out.Messages = append(out.Messages, WsMessage{
			ID:        m.ID,
			Raw:       m.Head.Raw,
			Direction: m.Head.Direction,
			Format:    m.Head.Format,
			Length:    m.Head.Length,
			CreatedAt: m.Head.CreatedAt,
		})
	}
	return out, nil
}

// SetWsDraft writes the entry's pending message. Caido requires editorState on
// every write, so an empty input reuses whatever the draft already holds (and
// an empty string for an entry that has never had a draft).
func SetWsDraft(
	ctx context.Context, client *caido.Client, entryID string, in WsDraftInput,
) (*WsDraft, error) {
	editorState := in.EditorState
	if editorState == "" {
		entry, err := GetWsEntry(ctx, client, entryID)
		if err != nil {
			return nil, err
		}
		if entry.EntryMissing {
			return nil, fmt.Errorf("replay entry %s: not found", entryID)
		}
		if !entry.IsWebSocket {
			return nil, fmt.Errorf(
				"replay entry %s is not a WebSocket entry", entryID,
			)
		}
		if entry.Draft != nil {
			editorState = entry.Draft.EditorState
		}
	}

	timeout := in.ServerTimeoutMs
	if timeout <= 0 {
		timeout = 5000
	}

	direction := strings.ToUpper(strings.TrimSpace(in.Direction))
	if direction == "" {
		direction = DirectionServer
	}
	format := strings.ToUpper(strings.TrimSpace(in.Format))
	if format == "" {
		format = "TEXT"
	}

	var data struct {
		UpdateReplayEntryDraft struct {
			Entry *struct {
				Typename string `json:"__typename"`
				Draft    *struct {
					Raw       string `json:"raw"`
					Direction string `json:"direction"`
					Format    string `json:"format"`
				} `json:"draft"`
			} `json:"entry"`
		} `json:"updateReplayEntryDraft"`
	}
	input := map[string]any{"ws": map[string]any{
		"raw":       in.Raw,
		"direction": direction,
		"format":    format,
		"settings":  map[string]any{"serverTimeoutMs": timeout},
		// Caido rejects a draft write without an editorState Blob.
		"editorState": editorState,
	}}
	if err := client.GraphQL.MakeRequest(ctx, &gql.Request{
		OpName:    "SetReplayEntryWsDraft",
		Query:     setWsDraftMutation,
		Variables: map[string]any{"id": entryID, "input": input},
	}, &gql.Response{Data: &data}); err != nil {
		return nil, fmt.Errorf("set ws draft on entry %s: %w", entryID, err)
	}
	entry := data.UpdateReplayEntryDraft.Entry
	if entry == nil || entry.Draft == nil {
		return nil, fmt.Errorf(
			"set ws draft on entry %s: Caido returned no draft "+
				"(is the entry a WebSocket entry?)", entryID,
		)
	}
	return &WsDraft{
		Raw:       entry.Draft.Raw,
		Direction: entry.Draft.Direction,
		Format:    entry.Draft.Format,
	}, nil
}

// ClearWsDraft removes the entry's pending message.
func ClearWsDraft(
	ctx context.Context, client *caido.Client, entryID string,
) error {
	var data struct {
		ClearReplayEntryDraft struct {
			Entry *struct {
				Typename string `json:"__typename"`
				ID       string `json:"id"`
			} `json:"entry"`
		} `json:"clearReplayEntryDraft"`
	}
	if err := client.GraphQL.MakeRequest(ctx, &gql.Request{
		OpName: "ClearReplayWsDraft",
		Query:  clearDraftMutation,
		Variables: map[string]any{
			"id": entryID, "kind": "WS",
		},
	}, &gql.Response{Data: &data}); err != nil {
		return fmt.Errorf("clear ws draft on entry %s: %w", entryID, err)
	}
	if data.ClearReplayEntryDraft.Entry == nil {
		return fmt.Errorf(
			"clear ws draft on entry %s: Caido returned no entry", entryID,
		)
	}
	return nil
}
