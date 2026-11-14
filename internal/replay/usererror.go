package replay

import "errors"

// Caido mutations report refusals through a per-operation error union.
// sdk-go's operations select only __typename from it, which is how a plan
// limit turned into "returned no session"; the hand-written operations in
// this package splice userErrorSelection in instead, so the code and the
// permission or cloud reason reach the caller. Only unions that contain
// PermissionDeniedUserError and CloudUserError may use it (stopReplayWsTasks'
// union does not, so it selects less).
const userErrorSelection = `__typename
      ... on UserError { code }
      ... on PermissionDeniedUserError { permissionReason: reason }
      ... on CloudUserError { cloudReason: reason }`

// Error-union member names the callers act on.
const (
	unknownIDTypename      = "UnknownIdUserError"
	taskInProgressTypename = "TaskInProgressUserError"
)

type userError struct {
	Typename         string `json:"__typename"`
	Code             string `json:"code"`
	PermissionReason string `json:"permissionReason"`
	CloudReason      string `json:"cloudReason"`
}

// Error makes a reported user error usable with %w and errors.As.
func (e *userError) Error() string { return e.String() }

// String renders the union member, its code and any permission or cloud
// reason, e.g. "PermissionDeniedUserError: PERMISSION_DENIED (ENTITLEMENT)".
func (e *userError) String() string {
	msg := e.Typename
	if e.Code != "" {
		msg += ": " + e.Code
	}
	if reason := e.PermissionReason + e.CloudReason; reason != "" {
		msg += " (" + reason + ")"
	}
	return msg
}

// describe is String plus a hint when the error is the plan's cap on how
// many sessions of kind may exist.
func (e *userError) describe(kind string) string {
	msg := e.String()
	if e.PermissionReason != "ENTITLEMENT" {
		return msg
	}
	switch kind {
	case "pipeline":
		// Seen live on 0.59.0: deleting one let the next create succeed.
		msg += "; the Caido plan's limit on pipeline sessions is reached, " +
			"delete an old pipeline session (caido_list_replay_sessions, " +
			"kind pipeline) or upgrade the plan"
	case "WS":
		// Seen live on 0.59.1: the plan allowed a single WS session.
		msg += "; the Caido plan's limit on WebSocket sessions is reached, " +
			"reuse one by passing its id as sessionId to caido_ws_connect " +
			"(caido_list_replay_sessions, kind ws), delete one, or upgrade the plan"
	}
	return msg
}

// describeErr is describe as an error value.
func (e *userError) describeErr(kind string) error {
	return errors.New(e.describe(kind))
}
