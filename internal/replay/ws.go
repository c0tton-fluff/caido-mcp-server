package replay

import (
	"bytes"
	"cmp"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"time"

	gql "github.com/Khan/genqlient/graphql"
	caido "github.com/caido-community/sdk-go"
)

// WebSocket replay (Caido 0.59). A WS session stores the upgrade handshake
// as an HTTP entry embedded in its WS entry. startReplayTask opens the
// socket and snapshots a new entry that carries the stream,
// sendReplayTaskMessage writes frames to the open task, and
// stopReplayWsTasks closes it. sdk-go has none of these operations, and its
// CreateSession drops the error union, so a plan's cap on WS sessions
// surfaced only as "create replay session returned no session".
//
// Behaviour observed live on 0.59.1 that the code below relies on:
//   - A message's direction names its destination: frames the client sent
//     are stored as SERVER, frames the server pushed as CLIENT. The
//     direction passed to sendReplayTaskMessage does not change routing.
//   - startReplayTask on a session whose task is still open fails with
//     TASK_IN_PROGRESS.
//   - Each startReplayTask creates a new entry; the session's activeEntry
//     follows it, so the stream stays readable after the task stops.
//   - updateReplayEntryDraft on the WS entry's HTTP entry id rewrites the
//     handshake, and the next startReplayTask sends the draft. That is how
//     a session is reused when the plan allows a single WS session.
//   - A refused connection leaves neither a handshake response nor an
//     entry error; the task just disappears from the task list.
//   - An idle socket with no keepalive traffic stayed open for the 4
//     minutes measured; Caido does not time it out on its own.

const (
	wsPollInit = 100 * time.Millisecond
	wsPollMax  = time.Second
	// The handshake usually lands within a second; poll it more tightly.
	wsHandshakePollMax = 250 * time.Millisecond
	// wsStartRetry bounds how long a start on a reused session retries
	// TASK_IN_PROGRESS while the previous task finishes.
	wsStartRetry = 3 * time.Second
	// wsAliveEvery spaces WSRead's open-connection checks: a close only
	// needs to be noticed eventually, not on every idle poll.
	wsAliveEvery = 2 * time.Second
	wsReadPage   = 100
	// wsMaxScan bounds the frames one WSRead scans, kept or skipped, so a
	// stream holding a flood of keep-alives cannot pin a call; the cursor
	// resumes where it stopped.
	wsMaxScan = 10000
)

// wsDirToServer is the stored direction of a frame the client sent. Caido
// names the destination, not the sender (see above).
const wsDirToServer = "SERVER"

const getWSSessionQuery = `query GetReplayWsSession($id: ID!) {
  replaySession(id: $id) {
    __typename
    id
    name
    ... on ReplaySessionWs {
      collection { id name }
      activeEntry {
        ... on ReplayEntryWs { http { id } stream { id } }
      }
    }
  }
}`

const listWSTasksQuery = `query ListReplayWsTasks {
  tasks {
    __typename
    id
    ... on ReplayTask {
      sessionKind
      replayEntry {
        session { id }
        ... on ReplayEntryWs { stream { id } }
      }
    }
  }
}`

const updateWSHandshakeMutation = `mutation UpdateReplayWsHandshake($id: ID!, $input: UpdateReplayEntryDraftInput!) {
  updateReplayEntryDraft(id: $id, input: $input) {
    entry { id }
  }
}`

const startWSTaskMutation = `mutation StartReplayWsTask($sessionId: ID!) {
  startReplayTask(sessionId: $sessionId) {
    task { id replayEntry { id } }
    error { ` + userErrorSelection + ` }
  }
}`

// getWSEntryQuery reads the handshake state and, in the same round trip,
// the task list, which is how a refused connection is noticed.
const getWSEntryQuery = `query GetReplayWsEntry($id: ID!) {
  replayEntry(id: $id, sessionKind: WS) {
    error
    ... on ReplayEntryWs {
      stream { id }
      http { request { response { statusCode raw } } }
    }
  }
  tasks { id }
}`

const sendWSMessageMutation = `mutation SendReplayWsMessage($task: ID!, $input: SendReplayTaskMessageInput!) {
  sendReplayTaskMessage(task: $task, input: $input) {
    message { ... on StreamWsMessage { id } }
    error { ` + userErrorSelection + ` }
  }
}`

const stopWSTasksMutation = `mutation StopReplayWsTasks($taskIds: [ID!]!) {
  stopReplayWsTasks(taskIds: $taskIds) {
    error { __typename ... on UserError { code } }
  }
}`

const readWSMessagesQuery = `query ReadReplayWsMessages($streamId: ID!, $after: String, $first: Int) {
  streamWsMessages(streamId: $streamId, after: $after, first: $first, order: {by: ID, ordering: ASC}) {
    pageInfo { hasNextPage }
    edges {
      cursor
      node { id head { direction format length raw } }
    }
  }
}`

// getHandshakeRequestQuery is a lighter request(id) than sdk-go's
// Requests.Get, which also fetches the response.
const getHandshakeRequestQuery = `query GetWsHandshakeRequest($id: ID!) {
  request(id: $id) { host port isTls raw }
}`

// ErrWSNotOpen is returned when a session has no open WebSocket task.
var ErrWSNotOpen = errors.New("no open WebSocket connection on this session")

// wsUserErr maps the error union of a call on a task: an unknown task ID
// means the connection is no longer open.
func wsUserErr(e *userError) error {
	if e.Typename == unknownIDTypename {
		return ErrWSNotOpen
	}
	return e
}

// wsSession is what WS replay needs from a session.
type wsSession struct {
	// handshakeID is the HTTP entry holding the upgrade request; empty for
	// a session created without one.
	handshakeID string
	// streamID is the stream of the latest connection, empty if it never
	// connected.
	streamID       string
	name           string
	collectionID   string
	collectionName string
}

// getWSSession reads a session and checks it is a WebSocket session.
func getWSSession(ctx context.Context, client *caido.Client, id string) (*wsSession, error) {
	var data struct {
		ReplaySession *struct {
			Typename    string    `json:"__typename"`
			Name        string    `json:"name"`
			Collection  *namedRef `json:"collection"`
			ActiveEntry *struct {
				HTTP *struct {
					ID string `json:"id"`
				} `json:"http"`
				Stream *struct {
					ID string `json:"id"`
				} `json:"stream"`
			} `json:"activeEntry"`
		} `json:"replaySession"`
	}
	if err := client.GraphQL.MakeRequest(ctx, &gql.Request{
		OpName:    "GetReplayWsSession",
		Query:     getWSSessionQuery,
		Variables: map[string]any{"id": id},
	}, &gql.Response{Data: &data}); err != nil {
		return nil, fmt.Errorf("read session %s: %w", id, err)
	}
	s := data.ReplaySession
	if s == nil {
		return nil, fmt.Errorf("replay session %s not found", id)
	}
	if kind := sessionKind(s.Typename); kind != KindWS {
		return nil, fmt.Errorf("replay session %s is a %s session, not a WebSocket one", id, kind)
	}
	out := &wsSession{name: s.Name}
	if c := s.Collection; c != nil {
		out.collectionID, out.collectionName = c.ID, c.Name
	}
	if e := s.ActiveEntry; e != nil {
		if e.HTTP != nil {
			out.handshakeID = e.HTTP.ID
		}
		if e.Stream != nil {
			out.streamID = e.Stream.ID
		}
	}
	return out, nil
}

// findWSTask returns the session's open task and its stream (empty while
// the handshake is pending), or an empty taskID when none is open.
func findWSTask(
	ctx context.Context, client *caido.Client, sessionID string,
) (taskID, streamID string, err error) {
	var data struct {
		Tasks []struct {
			Typename    string `json:"__typename"`
			ID          string `json:"id"`
			SessionKind string `json:"sessionKind"`
			ReplayEntry *struct {
				Session struct {
					ID string `json:"id"`
				} `json:"session"`
				Stream *struct {
					ID string `json:"id"`
				} `json:"stream"`
			} `json:"replayEntry"`
		} `json:"tasks"`
	}
	if err := client.GraphQL.MakeRequest(ctx, &gql.Request{
		OpName: "ListReplayWsTasks",
		Query:  listWSTasksQuery,
	}, &gql.Response{Data: &data}); err != nil {
		return "", "", fmt.Errorf("list tasks: %w", err)
	}
	for _, t := range data.Tasks {
		if t.Typename != "ReplayTask" || t.SessionKind != "WS" || t.ReplayEntry == nil ||
			t.ReplayEntry.Session.ID != sessionID {
			continue
		}
		if t.ReplayEntry.Stream != nil {
			streamID = t.ReplayEntry.Stream.ID
		}
		return t.ID, streamID, nil
	}
	return "", "", nil
}

// WSOpenTask returns the session's open task, empty when none is open.
func WSOpenTask(ctx context.Context, client *caido.Client, sessionID string) (string, error) {
	taskID, _, err := findWSTask(ctx, client, sessionID)
	return taskID, err
}

// WSState locates a WS session's connection.
type WSState struct {
	// TaskID is the open task, empty when no connection is open.
	TaskID string
	// StreamID is the open connection's stream, or else the latest one's;
	// empty if the session never connected or the handshake is pending.
	StreamID string
}

// WSSessionState finds the session's open task, falling back to its latest
// stream so frames stay readable after the connection closed.
func WSSessionState(
	ctx context.Context, client *caido.Client, sessionID string,
) (*WSState, error) {
	taskID, streamID, err := findWSTask(ctx, client, sessionID)
	if err != nil {
		return nil, err
	}
	if taskID != "" {
		return &WSState{TaskID: taskID, StreamID: streamID}, nil
	}
	s, err := getWSSession(ctx, client, sessionID)
	if err != nil {
		return nil, err
	}
	return &WSState{StreamID: s.streamID}, nil
}

// WSConnectOptions configures WSConnect.
type WSConnectOptions struct {
	// SessionID reuses an existing WS session: its handshake is replaced
	// and any open connection on it is closed. Empty creates one.
	SessionID    string
	CollectionID string
	Name         string
	// Raw is the plain-text upgrade request, CRLF line endings.
	Raw  string
	Conn caido.ReplayConnection
	// SkipOpen only stores the handshake, leaving the tab ready for a human
	// to click Connect.
	SkipOpen bool
	// Timeout bounds the wait for the handshake response and, on a reused
	// session, for its open connection to close. A zero Timeout leaves the
	// close bounded by wsCloseTimeout.
	Timeout time.Duration
	// ReuseOnLimit, when creating a session hits the plan's cap on WS
	// sessions and exactly one WS session exists, reuses that one instead
	// (moved into CollectionID and renamed to Name).
	ReuseOnLimit bool
}

// WSConnection is the outcome of WSConnect. A connection still waiting for
// its handshake when Timeout passed has neither StatusCode nor Error.
type WSConnection struct {
	SessionID string
	Created   bool
	// Reused reports that creating hit the plan's session cap and the only
	// WS session was reused (ReuseOnLimit).
	Reused bool
	// ClosedPrevious reports that a connection open on the reused session
	// was closed.
	ClosedPrevious bool
	// PreviousStreamID is the stream of the run a reused session showed
	// before: the tab now shows the new run, the old one stays readable in
	// WS history under this stream.
	PreviousStreamID string
	// MovedFrom is the collection a reused session was moved out of.
	MovedFrom string
	// Name, CollectionID and CollectionName describe where the session is
	// filed after the call; empty if reading it back failed.
	Name           string
	CollectionID   string
	CollectionName string
	StreamID       string
	// Open reports whether the connection was still open after the wait.
	Open bool
	// Error is the reason the connection failed before any response.
	Error       string
	StatusCode  int
	ResponseRaw string // base64
}

// WSConnect stores the handshake in a WS replay session (new or reused)
// and, unless SkipOpen, opens the socket and waits for the handshake.
func WSConnect(
	ctx context.Context, client *caido.Client, opts WSConnectOptions,
) (*WSConnection, error) {
	if opts.Raw == "" {
		return nil, fmt.Errorf("handshake request is empty")
	}
	conn := &WSConnection{SessionID: opts.SessionID}
	if opts.SessionID == "" {
		input := map[string]any{"kind": "WS", "requestSource": rawRequestSource(opts.Raw, opts.Conn)}
		if opts.CollectionID != "" {
			input["collectionId"] = opts.CollectionID
		}
		id, err := CreateNamedSession(ctx, client, input, opts.Name)
		if err == nil {
			conn.SessionID, conn.Created = id, true
		} else if only, err := sessionToReuse(ctx, client, err, opts.ReuseOnLimit); err != nil {
			return nil, err
		} else {
			conn.SessionID, conn.Reused = only, true
			opts.SessionID = only
		}
	}
	if !conn.Created {
		if err := reseedWSSession(ctx, client, opts, conn); err != nil {
			return conn, err
		}
	}
	if !opts.SkipOpen {
		// On a reused session a task that just ended (closed here, by the
		// server, or by a human) can still be finishing inside Caido.
		task, entry, err := startWSTask(ctx, client, conn.SessionID, !conn.Created)
		if err != nil {
			return conn, err
		}
		if err := waitHandshake(ctx, client, conn, task, entry, opts.Timeout); err != nil {
			return conn, err
		}
	}
	// A created or moved session's collection name is not known yet. Best
	// effort: where the traffic is filed is not worth failing a connection
	// that is already open.
	if conn.CollectionName == "" {
		if s, err := getWSSession(ctx, client, conn.SessionID); err == nil {
			conn.Name, conn.CollectionID, conn.CollectionName = s.name, s.collectionID, s.collectionName
		}
	}
	return conn, nil
}

// sessionToReuse decides what a failed create falls back to: with the plan's
// WS session cap reached, reuse=true and exactly one WS session, that
// session's ID; otherwise the error to report.
func sessionToReuse(ctx context.Context, client *caido.Client, err error, reuse bool) (string, error) {
	err = fmt.Errorf("create ws session: %w", err)
	if !errors.Is(err, ErrSessionLimit) {
		return "", err
	}
	ids, lerr := wsSessionIDs(ctx, client)
	switch {
	case lerr != nil:
		return "", fmt.Errorf("%w (listing WS sessions failed: %w)", err, lerr)
	case len(ids) == 1 && reuse:
		return ids[0], nil
	case len(ids) == 1:
		return "", fmt.Errorf("%w; or pass reuse=true to reuse the only WS session %s "+
			"(moved into collectionId, renamed to name)", err, ids[0])
	case reuse:
		return "", fmt.Errorf("%w; reuse=true needs exactly one WS session to exist", err)
	}
	return "", err
}

// wsSessionIDs lists the IDs of the project's WS replay sessions.
func wsSessionIDs(ctx context.Context, client *caido.Client) ([]string, error) {
	all, err := ListSessions(ctx, client)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, s := range all {
		if s.Kind == KindWS {
			ids = append(ids, s.ID)
		}
	}
	return ids, nil
}

// wsCloseTimeout bounds a stop. Caido can leave stopReplayWsTasks
// unanswered for a socket the server keeps open.
const wsCloseTimeout = 10 * time.Second

// reseedWSSession replaces the session's handshake, applies name and
// collection, and then closes any open connection on it, last, so a failed
// step leaves a live socket alone. Drafting the handshake does not touch
// the running task (each start snapshots a new entry). It records in conn
// the run being replaced, the collection left and whether a connection
// was closed.
func reseedWSSession(
	ctx context.Context, client *caido.Client, opts WSConnectOptions, conn *WSConnection,
) error {
	s, err := getWSSession(ctx, client, opts.SessionID)
	if err != nil {
		return err
	}
	if s.handshakeID == "" {
		retry := "call caido_ws_connect without sessionId"
		if conn.Reused {
			retry = "retry"
		}
		return fmt.Errorf(
			"ws session %s was created empty and has no handshake to replace; "+
				"delete it (caido_delete_replay_sessions) and %s", opts.SessionID, retry,
		)
	}
	conn.Name, conn.CollectionID, conn.CollectionName = s.name, s.collectionID, s.collectionName

	var updated struct {
		UpdateReplayEntryDraft struct {
			Entry *struct {
				ID string `json:"id"`
			} `json:"entry"`
		} `json:"updateReplayEntryDraft"`
	}
	if err := client.GraphQL.MakeRequest(ctx, &gql.Request{
		OpName: "UpdateReplayWsHandshake",
		Query:  updateWSHandshakeMutation,
		Variables: map[string]any{
			"id": s.handshakeID,
			"input": map[string]any{"http": map[string]any{
				"raw":         base64.StdEncoding.EncodeToString([]byte(opts.Raw)),
				"connection":  connectionInput(opts.Conn),
				"settings":    map[string]any{"placeholders": []any{}},
				"editorState": "",
			}},
		},
	}, &gql.Response{Data: &updated}); err != nil {
		return fmt.Errorf("replace handshake of session %s: %w", opts.SessionID, err)
	}
	if updated.UpdateReplayEntryDraft.Entry == nil {
		return fmt.Errorf("replace handshake of session %s: entry not found", opts.SessionID)
	}

	if opts.Name != "" {
		if _, _, err := RenameSession(ctx, client, opts.SessionID, opts.Name); err != nil {
			return fmt.Errorf("rename session %s: %w", opts.SessionID, err)
		}
		conn.Name = opts.Name
	}
	if opts.CollectionID != "" {
		if _, err := MoveSession(ctx, client, opts.SessionID, opts.CollectionID); err != nil {
			return fmt.Errorf("move session %s: %w", opts.SessionID, err)
		}
		if s.collectionID != opts.CollectionID {
			conn.MovedFrom = s.collectionID
			conn.CollectionID, conn.CollectionName = opts.CollectionID, ""
		}
	}
	closeTimeout := cmp.Or(opts.Timeout, wsCloseTimeout)
	res, err := closeWSTask(ctx, client, opts.SessionID, closeTimeout)
	conn.ClosedPrevious = res.Closed
	if err != nil {
		return err
	}
	if res.TimedOut {
		return fmt.Errorf(
			"the Caido Replay task did not close its open connection within %v; "+
				"close it in the Replay tab or retry", closeTimeout,
		)
	}
	if !opts.SkipOpen {
		// Only now is the tab about to follow a new entry; on an earlier
		// failure it still shows this stream.
		conn.PreviousStreamID = s.streamID
	}
	return nil
}

// startWSTask starts the session's task and returns its task and entry IDs.
// Right after a previous task ended Caido can briefly still report
// TASK_IN_PROGRESS, so with retryBusy that case is retried for wsStartRetry.
func startWSTask(
	ctx context.Context, client *caido.Client, sessionID string, retryBusy bool,
) (string, string, error) {
	deadline := time.Now().Add(wsStartRetry)
	interval := wsPollInit
	for {
		var started struct {
			StartReplayTask struct {
				Task *struct {
					ID          string `json:"id"`
					ReplayEntry struct {
						ID string `json:"id"`
					} `json:"replayEntry"`
				} `json:"task"`
				Error *userError `json:"error"`
			} `json:"startReplayTask"`
		}
		if err := client.GraphQL.MakeRequest(ctx, &gql.Request{
			OpName:    "StartReplayWsTask",
			Query:     startWSTaskMutation,
			Variables: map[string]any{"sessionId": sessionID},
		}, &gql.Response{Data: &started}); err != nil {
			return "", "", fmt.Errorf("open ws session %s: %w", sessionID, err)
		}
		st := started.StartReplayTask
		if st.Error != nil {
			if st.Error.Typename == taskInProgressTypename && retryBusy && time.Now().Before(deadline) {
				if err := sleepCtx(ctx, interval); err != nil {
					return "", "", err
				}
				interval = min(interval*2, wsPollMax)
				continue
			}
			return "", "", fmt.Errorf("open ws session %s: %w", sessionID, st.Error)
		}
		if st.Task == nil || st.Task.ReplayEntry.ID == "" {
			return "", "", fmt.Errorf("open ws session %s returned no task", sessionID)
		}
		return st.Task.ID, st.Task.ReplayEntry.ID, nil
	}
}

const wsNoHandshakeError = "connection closed before any handshake response " +
	"(refused, DNS or TLS failure; Caido records no reason)"

// waitHandshake polls the task's entry until the upgrade response or an
// entry error arrives, the task ends without either, or timeout passes,
// recording the outcome in conn.
func waitHandshake(
	ctx context.Context, client *caido.Client, conn *WSConnection,
	taskID, entryID string, timeout time.Duration,
) error {
	deadline := time.Now().Add(timeout)
	interval := wsPollInit
	missed := 0
	for {
		if err := readHandshake(ctx, client, conn, taskID, entryID); err != nil {
			return err
		}
		if conn.StatusCode != 0 || conn.Error != "" {
			return nil
		}
		// No response yet. The task must be missing on two reads a poll
		// apart before the connection counts as failed, so a task Caido has
		// not listed yet is not mistaken for one that ended; at the deadline
		// one miss is enough.
		timedOut := !time.Now().Before(deadline)
		missed++
		if conn.Open {
			missed = 0
		}
		if missed >= 2 || (missed == 1 && timedOut) {
			conn.Error = wsNoHandshakeError
			return nil
		}
		if timedOut {
			return nil
		}
		if err := sleepCtx(ctx, interval); err != nil {
			return err
		}
		interval = min(interval*2, wsHandshakePollMax)
	}
}

// readHandshake reads the task's entry and the task list in one round trip
// and records the handshake state and whether the task is still listed.
func readHandshake(
	ctx context.Context, client *caido.Client, conn *WSConnection, taskID, entryID string,
) error {
	var data struct {
		ReplayEntry *struct {
			Error  *string `json:"error"`
			Stream *struct {
				ID string `json:"id"`
			} `json:"stream"`
			HTTP *struct {
				Request *struct {
					Response *struct {
						StatusCode int    `json:"statusCode"`
						Raw        string `json:"raw"`
					} `json:"response"`
				} `json:"request"`
			} `json:"http"`
		} `json:"replayEntry"`
		Tasks []idRef `json:"tasks"`
	}
	if err := client.GraphQL.MakeRequest(ctx, &gql.Request{
		OpName:    "GetReplayWsEntry",
		Query:     getWSEntryQuery,
		Variables: map[string]any{"id": entryID},
	}, &gql.Response{Data: &data}); err != nil {
		return fmt.Errorf("read ws entry %s: %w", entryID, err)
	}
	e := data.ReplayEntry
	if e == nil {
		return fmt.Errorf("ws entry %s not found", entryID)
	}
	conn.Open = slices.ContainsFunc(data.Tasks, func(t idRef) bool { return t.ID == taskID })
	if e.Stream != nil {
		conn.StreamID = e.Stream.ID
	}
	if e.HTTP != nil && e.HTTP.Request != nil && e.HTTP.Request.Response != nil {
		conn.StatusCode = e.HTTP.Request.Response.StatusCode
		conn.ResponseRaw = e.HTTP.Request.Response.Raw
	}
	if e.Error != nil {
		conn.Error = *e.Error
	}
	return nil
}

// WSFrame is one frame to send. Format is TEXT or BINARY.
type WSFrame struct {
	Format string
	Data   []byte
}

// WSSend writes frames in order on the open task, pausing delay between
// them. It returns how many frames were sent and the stored message IDs
// Caido reported for them, also when it fails part way.
func WSSend(
	ctx context.Context, client *caido.Client,
	taskID string, frames []WSFrame, delay time.Duration,
) (int, []string, error) {
	ids := make([]string, 0, len(frames))
	for i, f := range frames {
		if i > 0 && delay > 0 {
			if err := sleepCtx(ctx, delay); err != nil {
				return i, ids, err
			}
		}
		var sent struct {
			SendReplayTaskMessage struct {
				Message *struct {
					ID string `json:"id"`
				} `json:"message"`
				Error *userError `json:"error"`
			} `json:"sendReplayTaskMessage"`
		}
		if err := client.GraphQL.MakeRequest(ctx, &gql.Request{
			OpName: "SendReplayWsMessage",
			Query:  sendWSMessageMutation,
			Variables: map[string]any{
				"task": taskID,
				"input": map[string]any{"ws": map[string]any{
					"direction": wsDirToServer,
					"format":    f.Format,
					"data":      base64.StdEncoding.EncodeToString(f.Data),
				}},
			},
		}, &gql.Response{Data: &sent}); err != nil {
			return i, ids, fmt.Errorf("send frame %d: %w", i, err)
		}
		p := sent.SendReplayTaskMessage
		if p.Error != nil {
			return i, ids, fmt.Errorf("send frame %d: %w", i, wsUserErr(p.Error))
		}
		if p.Message != nil {
			ids = append(ids, p.Message.ID)
		}
	}
	return len(frames), ids, nil
}

// WSClose stops a WS task. Caido closes the socket without a status code.
func WSClose(ctx context.Context, client *caido.Client, taskID string) error {
	var stopped struct {
		StopReplayWsTasks struct {
			Error *userError `json:"error"`
		} `json:"stopReplayWsTasks"`
	}
	if err := client.GraphQL.MakeRequest(ctx, &gql.Request{
		OpName:    "StopReplayWsTasks",
		Query:     stopWSTasksMutation,
		Variables: map[string]any{"taskIds": []string{taskID}},
	}, &gql.Response{Data: &stopped}); err != nil {
		return fmt.Errorf("close ws task %s: %w", taskID, err)
	}
	if e := stopped.StopReplayWsTasks.Error; e != nil {
		return fmt.Errorf("close ws task %s: %w", taskID, wsUserErr(e))
	}
	return nil
}

// WSCloseResult is the outcome of closing a session's connection.
type WSCloseResult struct {
	// Closed reports that an open connection was closed.
	Closed bool
	// TimedOut reports that Caido did not answer the stop within the
	// timeout and the connection is still listed as open, or the task list
	// could not be read to tell.
	TimedOut bool
}

// closeWSTask closes the session's open connection, if any, waiting at most
// timeout for Caido to answer. A task that ends on its own meanwhile is not
// an error.
func closeWSTask(
	ctx context.Context, client *caido.Client, sessionID string, timeout time.Duration,
) (WSCloseResult, error) {
	// The lookup and the stop share one deadline: a Caido that leaves the
	// stop unanswered may not answer the lookup either.
	opCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	// ours reports that our deadline, not the caller's context, ended a call.
	ours := func(c context.Context) bool { return c.Err() != nil && ctx.Err() == nil }

	taskID, _, err := findWSTask(opCtx, client, sessionID)
	switch {
	case err != nil && ours(opCtx):
		return WSCloseResult{TimedOut: true}, nil
	case err != nil || taskID == "":
		return WSCloseResult{}, err
	}
	err = WSClose(opCtx, client, taskID)
	switch {
	case err == nil:
		return WSCloseResult{Closed: true}, nil
	case errors.Is(err, ErrWSNotOpen):
		return WSCloseResult{}, nil
	case !ours(opCtx):
		return WSCloseResult{}, err
	}
	// The stop went unanswered; it may still have taken effect, so ask the
	// task list (bounded too) whether the task is gone.
	checkCtx, cancelCheck := context.WithTimeout(ctx, timeout)
	defer cancelCheck()
	still, _, err := findWSTask(checkCtx, client, sessionID)
	switch {
	case err != nil && ctx.Err() != nil:
		return WSCloseResult{}, ctx.Err()
	case err == nil && still != taskID:
		return WSCloseResult{Closed: true}, nil
	}
	return WSCloseResult{TimedOut: true}, nil
}

// WSCloseSession closes a WS session's open connection, waiting at most
// timeout for Caido to answer.
func WSCloseSession(
	ctx context.Context, client *caido.Client, sessionID string, timeout time.Duration,
) (WSCloseResult, error) {
	res, err := closeWSTask(ctx, client, sessionID, timeout)
	if err != nil || res.Closed || res.TimedOut {
		return res, err
	}
	// Nothing was open: still report a session that is missing or not WS.
	_, err = getWSSession(ctx, client, sessionID)
	return res, err
}

// WSMessage is one stored frame.
type WSMessage struct {
	ID string
	// Sent is true for frames the client sent, false for frames the server
	// pushed (Caido's destination-named direction, translated).
	Sent   bool
	Format string
	Length int
	Data   []byte
	// Truncated reports Data was cut at WSReadOptions.MaxBody.
	Truncated bool
}

// WSReadOptions configures WSRead.
type WSReadOptions struct {
	// After is the cursor a previous WSRead returned; empty reads from the
	// first frame of the stream.
	After string
	// Wait is how long to wait for a frame that satisfies the stop rule.
	Wait time.Duration
	// UntilContains stops once a kept frame contains this text, and
	// UntilCount once this many frames were kept; set at most one. Without
	// either, WSRead returns once a scan kept any frame, with every frame
	// stored so far (up to Limit).
	UntilContains string
	UntilCount    int
	SkipBodies    map[string]bool
	IncludeSent   bool
	Limit         int
	// MaxBody caps the bytes kept of each frame (0 keeps all). The stop and
	// skip rules still see the whole body; the cap only bounds what a read
	// holds, since a server chooses its frame sizes.
	MaxBody int
	// Alive, when set, is consulted (at most every wsAliveEvery) while
	// nothing new arrives; WSRead returns once it reports the connection
	// closed.
	Alive func(context.Context) (bool, error)
}

// WSReadResult is the outcome of WSRead.
type WSReadResult struct {
	Messages []WSMessage
	// Cursor resumes after the last frame scanned (kept or skipped).
	Cursor string
	// SkippedSent counts client frames dropped (IncludeSent off),
	// SkippedBodies frames dropped by SkipBodies, SkippedControl PING and
	// PONG frames.
	SkippedSent    int
	SkippedBodies  int
	SkippedControl int
	// Matched reports the stop rule was met; false means Wait passed, the
	// limit or the scan cap was hit, or the connection closed first.
	Matched bool
	// Closed reports Alive said the connection closed.
	Closed bool
	// More reports the read stopped at wsMaxScan with frames left to scan.
	More bool
}

// wsReader holds one WSRead call's progress across polls.
type wsReader struct {
	client   *caido.Client
	streamID string
	opts     WSReadOptions
	needle   []byte
	scanned  int // frames scanned by this WSRead, across polls
	res      WSReadResult
	hasMatch bool // a kept frame contained UntilContains
}

// met reports whether the stop rule holds.
func (r *wsReader) met() bool {
	switch {
	case r.opts.UntilCount > 0:
		return len(r.res.Messages) >= r.opts.UntilCount
	case r.opts.UntilContains != "":
		return r.hasMatch
	default:
		return len(r.res.Messages) > 0
	}
}

// wsNode is one stored frame as streamWsMessages returns it.
type wsNode struct {
	ID   string `json:"id"`
	Head struct {
		Direction string `json:"direction"`
		Format    string `json:"format"`
		Length    int    `json:"length"`
		Raw       string `json:"raw"`
	} `json:"head"`
}

// keep converts a stored frame, or counts it as skipped and reports false.
// Frames dropped on direction or format are not decoded.
func (r *wsReader) keep(n wsNode) (WSMessage, bool) {
	h := n.Head
	sent := h.Direction == wsDirToServer
	switch {
	case h.Format == "PING" || h.Format == "PONG":
		r.res.SkippedControl++
		return WSMessage{}, false
	case sent && !r.opts.IncludeSent:
		r.res.SkippedSent++
		return WSMessage{}, false
	}
	body, err := base64.StdEncoding.DecodeString(h.Raw)
	if err != nil {
		body = []byte(h.Raw)
	}
	if r.opts.SkipBodies[string(body)] {
		r.res.SkippedBodies++
		return WSMessage{}, false
	}
	return WSMessage{ID: n.ID, Sent: sent, Format: h.Format, Length: h.Length, Data: body}, true
}

// scan reads every frame stored after the cursor. It stops early, with
// stop set, at the frame that meets an until rule, fills Limit or reaches
// wsMaxScan, and reports how many frames it scanned.
func (r *wsReader) scan(ctx context.Context) (stop bool, scanned int, err error) {
	hasRule := r.opts.UntilCount > 0 || r.opts.UntilContains != ""
	for {
		vars := map[string]any{"streamId": r.streamID, "first": wsReadPage}
		if r.res.Cursor != "" {
			vars["after"] = r.res.Cursor
		}
		var data struct {
			StreamWsMessages struct {
				PageInfo struct {
					HasNextPage bool `json:"hasNextPage"`
				} `json:"pageInfo"`
				Edges []struct {
					Cursor string `json:"cursor"`
					Node   wsNode `json:"node"`
				} `json:"edges"`
			} `json:"streamWsMessages"`
		}
		if err := r.client.GraphQL.MakeRequest(ctx, &gql.Request{
			OpName:    "ReadReplayWsMessages",
			Query:     readWSMessagesQuery,
			Variables: vars,
		}, &gql.Response{Data: &data}); err != nil {
			return false, scanned, fmt.Errorf("read stream %s: %w", r.streamID, err)
		}
		page := data.StreamWsMessages
		for _, e := range page.Edges {
			r.res.Cursor = e.Cursor
			scanned++
			r.scanned++
			if m, ok := r.keep(e.Node); ok {
				if r.needle != nil && bytes.Contains(m.Data, r.needle) {
					r.hasMatch = true
				}
				if limit := r.opts.MaxBody; limit > 0 && len(m.Data) > limit {
					// Clone so the full decoded body can be freed.
					m.Data, m.Truncated = bytes.Clone(m.Data[:limit]), true
				}
				r.res.Messages = append(r.res.Messages, m)
			}
			// An until rule stops at the frame that meets it; without one,
			// everything already stored is returned (up to Limit).
			if (hasRule && r.met()) || (r.opts.Limit > 0 && len(r.res.Messages) >= r.opts.Limit) {
				return true, scanned, nil
			}
			if r.scanned >= wsMaxScan {
				r.res.More = true
				return true, scanned, nil
			}
		}
		if !page.PageInfo.HasNextPage || len(page.Edges) == 0 {
			return false, scanned, nil
		}
	}
}

// WSRead long-polls a stream for frames after opts.After. PING and PONG
// frames are always skipped.
func WSRead(
	ctx context.Context, client *caido.Client, streamID string, opts WSReadOptions,
) (*WSReadResult, error) {
	r := &wsReader{client: client, streamID: streamID, opts: opts}
	r.res.Cursor = opts.After
	if opts.UntilContains != "" {
		r.needle = []byte(opts.UntilContains)
	}
	deadline := time.Now().Add(opts.Wait)
	interval := wsPollInit
	var lastAlive time.Time
	for {
		stop, scanned, err := r.scan(ctx)
		if err != nil {
			return &r.res, err
		}
		if stop || r.met() || !time.Now().Before(deadline) {
			r.res.Matched = r.met()
			return &r.res, nil
		}
		if scanned == 0 && opts.Alive != nil && time.Since(lastAlive) >= wsAliveEvery {
			lastAlive = time.Now()
			alive, err := opts.Alive(ctx)
			if err != nil {
				return &r.res, err
			}
			if !alive {
				// One last scan for frames stored just before the close.
				r.res.Closed = true
				_, _, err := r.scan(ctx)
				r.res.Matched = r.met()
				return &r.res, err
			}
		}
		if err := sleepCtx(ctx, interval); err != nil {
			return &r.res, err
		}
		interval = min(interval*2, wsPollMax)
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// A Stream has no link back to the replay session that opened it, so
// WSStreamSessions walks the WS sessions' entries instead, newest first:
// the streams looked up are usually recent runs, so the walk can stop
// early.
const listWSSessionStreamsQuery = `query ListReplayWsSessionStreams($id: ID!, $after: String, $first: Int) {
  replaySession(id: $id) {
    ... on ReplaySessionWs {
      entries(first: $first, after: $after, order: {by: ID, ordering: DESC}) {
        pageInfo { hasNextPage endCursor }
        nodes { ... on ReplayEntryWs { stream { id } } }
      }
    }
  }
}`

type wsSessionStreamsResp struct {
	ReplaySession *struct {
		Entries *struct {
			PageInfo pageInfo `json:"pageInfo"`
			Nodes    []struct {
				Stream *idRef `json:"stream"`
			} `json:"nodes"`
		} `json:"entries"`
	} `json:"replaySession"`
}

// WSStreamSessions maps each of the given stream IDs that a WS replay
// session opened to that session's ID. Streams not opened from Replay are
// left out. On error it returns what it mapped so far.
func WSStreamSessions(
	ctx context.Context, client *caido.Client, streamIDs []string,
) (map[string]string, error) {
	out := make(map[string]string, len(streamIDs))
	if len(streamIDs) == 0 {
		return out, nil
	}
	want := make(map[string]bool, len(streamIDs))
	for _, id := range streamIDs {
		want[id] = true
	}
	ids, err := wsSessionIDs(ctx, client)
	if err != nil {
		return out, err
	}
	for _, id := range ids {
		if err := mapSessionStreams(ctx, client, id, want, out); err != nil {
			return out, err
		}
		if len(out) == len(want) {
			break
		}
	}
	return out, nil
}

// mapSessionStreams pages through one WS session's entries, recording in
// out each wanted stream it opened, until every wanted stream is mapped.
func mapSessionStreams(
	ctx context.Context, client *caido.Client, sessionID string,
	want map[string]bool, out map[string]string,
) error {
	var after *string
	for {
		var data wsSessionStreamsResp
		if err := client.GraphQL.MakeRequest(ctx, &gql.Request{
			OpName:    "ListReplayWsSessionStreams",
			Query:     listWSSessionStreamsQuery,
			Variables: map[string]any{"id": sessionID, "after": after, "first": listPageSize},
		}, &gql.Response{Data: &data}); err != nil {
			return fmt.Errorf("list entries of session %s: %w", sessionID, err)
		}
		if data.ReplaySession == nil || data.ReplaySession.Entries == nil {
			return nil
		}
		e := data.ReplaySession.Entries
		for _, n := range e.Nodes {
			if n.Stream != nil && want[n.Stream.ID] {
				out[n.Stream.ID] = sessionID
			}
		}
		if len(out) == len(want) || !e.PageInfo.HasNextPage || e.PageInfo.EndCursor == nil {
			return nil
		}
		after = e.PageInfo.EndCursor
	}
}

// HandshakeFromRequest loads a request from history (typically a captured
// WebSocket upgrade) as a plain-text handshake and its connection.
func HandshakeFromRequest(
	ctx context.Context, client *caido.Client, id string,
) (string, caido.ReplayConnection, error) {
	var data struct {
		Request *struct {
			Host  string `json:"host"`
			Port  int    `json:"port"`
			IsTLS bool   `json:"isTls"`
			Raw   string `json:"raw"`
		} `json:"request"`
	}
	if err := client.GraphQL.MakeRequest(ctx, &gql.Request{
		OpName:    "GetWsHandshakeRequest",
		Query:     getHandshakeRequestQuery,
		Variables: map[string]any{"id": id},
	}, &gql.Response{Data: &data}); err != nil {
		return "", caido.ReplayConnection{}, fmt.Errorf("read request %s: %w", id, err)
	}
	r := data.Request
	if r == nil {
		return "", caido.ReplayConnection{}, fmt.Errorf("request %s not found", id)
	}
	raw, err := base64.StdEncoding.DecodeString(r.Raw)
	if err != nil {
		return "", caido.ReplayConnection{}, fmt.Errorf("request %s: undecodable raw: %w", id, err)
	}
	return string(raw), caido.ReplayConnection{Host: r.Host, Port: r.Port, IsTLS: r.IsTLS}, nil
}
