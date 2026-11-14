package replay

import (
	"errors"
	"testing"
)

// A plan-cap refusal matches ErrSessionLimit and still exposes the
// underlying user error; any other refusal matches neither.
func TestDescribeErr_SessionLimit(t *testing.T) {
	limit := (&userError{
		Typename: "PermissionDeniedUserError", Code: "PERMISSION_DENIED", PermissionReason: "ENTITLEMENT",
	}).describeErr("WS")
	if !errors.Is(limit, ErrSessionLimit) {
		t.Fatalf("want ErrSessionLimit, got %v", limit)
	}
	var ue *userError
	if !errors.As(limit, &ue) || ue.Code != "PERMISSION_DENIED" {
		t.Fatalf("want the user error reachable with errors.As, got %v", limit)
	}

	other := (&userError{Typename: "OtherUserError", Code: "OTHER"}).describeErr("WS")
	if errors.Is(other, ErrSessionLimit) || errors.As(other, &ue) {
		t.Fatalf("want a non-cap refusal to match neither, got %v", other)
	}
}
