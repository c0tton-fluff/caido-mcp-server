package replay

import (
	"context"
	"fmt"

	gql "github.com/Khan/genqlient/graphql"
	caido "github.com/caido-community/sdk-go"
)

// Rename and move return the ReplaySession interface, which sdk-go's
// decoders reject for Pipeline sessions (see list.go): on a live 0.59.0
// server the rename of a fresh pipeline session failed with "unexpected
// concrete type" even though the server applied it. These keep sdk-go's
// operation names and decode the session as a plain struct.

const renameSessionMutation = `mutation RenameReplaySession($id: ID!, $name: String!) {
  renameReplaySession(id: $id, name: $name) {
    session { __typename id name }
  }
}`

const moveSessionMutation = `mutation MoveReplaySession($id: ID!, $collectionId: ID!) {
  moveReplaySession(id: $id, collectionId: $collectionId) {
    session { __typename id }
  }
}`

type sessionRef struct {
	Typename string `json:"__typename"`
	ID       string `json:"id"`
	Name     string `json:"name"`
}

// RenameSession renames a replay session of any kind and returns its ID
// and new name.
func RenameSession(
	ctx context.Context, client *caido.Client, id, name string,
) (string, string, error) {
	var data struct {
		RenameReplaySession struct {
			Session *sessionRef `json:"session"`
		} `json:"renameReplaySession"`
	}
	if err := client.GraphQL.MakeRequest(ctx, &gql.Request{
		OpName:    "RenameReplaySession",
		Query:     renameSessionMutation,
		Variables: map[string]any{"id": id, "name": name},
	}, &gql.Response{Data: &data}); err != nil {
		return "", "", err
	}
	s := data.RenameReplaySession.Session
	if s == nil {
		return "", "", fmt.Errorf("session %s not found", id)
	}
	return s.ID, s.Name, nil
}

// MoveSession moves a replay session of any kind into collectionID.
func MoveSession(
	ctx context.Context, client *caido.Client, id, collectionID string,
) (string, error) {
	var data struct {
		MoveReplaySession struct {
			Session *sessionRef `json:"session"`
		} `json:"moveReplaySession"`
	}
	if err := client.GraphQL.MakeRequest(ctx, &gql.Request{
		OpName:    "MoveReplaySession",
		Query:     moveSessionMutation,
		Variables: map[string]any{"id": id, "collectionId": collectionID},
	}, &gql.Response{Data: &data}); err != nil {
		return "", err
	}
	s := data.MoveReplaySession.Session
	if s == nil {
		return "", fmt.Errorf("session %s not found", id)
	}
	return s.ID, nil
}
