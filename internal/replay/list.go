package replay

import (
	"context"
	"fmt"
	"strings"

	gql "github.com/Khan/genqlient/graphql"
	caido "github.com/caido-community/sdk-go"
)

// Listing goes through hand-written queries instead of sdk-go because the
// generated decoders reject any __typename they were not generated for.
// Caido 0.58 added Pipeline sessions (ReplaySessionHttpOnePipeline), so a
// single pipeline session in the project made every sdk-go session list
// fail with "unexpected concrete type". Decoding into plain structs keeps
// unknown kinds as summaries instead.
//
// The fragments name only ReplaySessionHttp and ReplaySessionWs, which
// exist since 0.57, so the queries still validate on older servers. A
// pipeline session therefore lists without its collection or active entry.

const listPageSize = 100

const listSessionsQuery = `query ListReplaySessionsTolerant($first: Int, $after: String) {
  replaySessions(first: $first, after: $after) {
    edges {
      node {
        __typename
        id
        name
        ... on ReplaySessionHttp {
          activeEntry { id }
          collection { id name }
        }
        ... on ReplaySessionWs {
          activeEntry { id }
          collection { id name }
        }
      }
    }
    pageInfo { hasNextPage endCursor }
  }
}`

const listCollectionsQuery = `query ListReplaySessionCollectionsTolerant($first: Int, $after: String) {
  replaySessionCollections(first: $first, after: $after) {
    edges {
      node { id name }
    }
    pageInfo { hasNextPage endCursor }
  }
}`

// Session kinds as reported in summaries.
const (
	KindHTTP     = "http"
	KindWS       = "ws"
	KindPipeline = "pipeline"
)

// SessionSummary is one replay session as listed.
type SessionSummary struct {
	ID             string
	Name           string
	Kind           string // KindHTTP, KindWS, KindPipeline, or the raw __typename
	ActiveEntryID  string // empty when none or not exposed for this kind
	CollectionID   string
	CollectionName string
}

// CollectionSummary is one replay collection as listed.
type CollectionSummary struct {
	ID   string
	Name string
}

type pageInfo struct {
	HasNextPage bool    `json:"hasNextPage"`
	EndCursor   *string `json:"endCursor"`
}

type idRef struct {
	ID string `json:"id"`
}

type namedRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type listSessionsResp struct {
	ReplaySessions struct {
		Edges []struct {
			Node *struct {
				Typename    string    `json:"__typename"`
				ID          string    `json:"id"`
				Name        string    `json:"name"`
				ActiveEntry *idRef    `json:"activeEntry"`
				Collection  *namedRef `json:"collection"`
			} `json:"node"`
		} `json:"edges"`
		PageInfo pageInfo `json:"pageInfo"`
	} `json:"replaySessions"`
}

type listCollectionsResp struct {
	ReplaySessionCollections struct {
		Edges []struct {
			Node *namedRef `json:"node"`
		} `json:"edges"`
		PageInfo pageInfo `json:"pageInfo"`
	} `json:"replaySessionCollections"`
}

// sessionKind maps a ReplaySession __typename to a short kind.
func sessionKind(typename string) string {
	switch typename {
	case "ReplaySessionHttp":
		return KindHTTP
	case "ReplaySessionWs":
		return KindWS
	case "ReplaySessionHttpOnePipeline":
		return KindPipeline
	}
	return typename
}

// ListSessions returns every replay session, following pagination, and
// tolerates session kinds this build does not know.
func ListSessions(
	ctx context.Context, client *caido.Client,
) ([]SessionSummary, error) {
	var out []SessionSummary
	var after *string
	for {
		var data listSessionsResp
		if err := client.GraphQL.MakeRequest(ctx, &gql.Request{
			OpName:    "ListReplaySessionsTolerant",
			Query:     listSessionsQuery,
			Variables: map[string]any{"first": listPageSize, "after": after},
		}, &gql.Response{Data: &data}); err != nil {
			return nil, fmt.Errorf("list replay sessions: %w", err)
		}
		conn := data.ReplaySessions
		for _, e := range conn.Edges {
			n := e.Node
			if n == nil {
				continue
			}
			s := SessionSummary{ID: n.ID, Name: n.Name, Kind: sessionKind(n.Typename)}
			if n.ActiveEntry != nil {
				s.ActiveEntryID = n.ActiveEntry.ID
			}
			if n.Collection != nil {
				s.CollectionID, s.CollectionName = n.Collection.ID, n.Collection.Name
			}
			out = append(out, s)
		}
		if !conn.PageInfo.HasNextPage || conn.PageInfo.EndCursor == nil {
			return out, nil
		}
		after = conn.PageInfo.EndCursor
	}
}

// ListCollections returns every replay collection, following pagination.
// It does not select the sessions inside each collection, which is what
// made the sdk-go variant fail on pipeline sessions.
func ListCollections(
	ctx context.Context, client *caido.Client,
) ([]CollectionSummary, error) {
	var out []CollectionSummary
	var after *string
	for {
		var data listCollectionsResp
		if err := client.GraphQL.MakeRequest(ctx, &gql.Request{
			OpName:    "ListReplaySessionCollectionsTolerant",
			Query:     listCollectionsQuery,
			Variables: map[string]any{"first": listPageSize, "after": after},
		}, &gql.Response{Data: &data}); err != nil {
			return nil, fmt.Errorf("list replay collections: %w", err)
		}
		conn := data.ReplaySessionCollections
		for _, e := range conn.Edges {
			if e.Node != nil {
				out = append(out, CollectionSummary{ID: e.Node.ID, Name: e.Node.Name})
			}
		}
		if !conn.PageInfo.HasNextPage || conn.PageInfo.EndCursor == nil {
			return out, nil
		}
		after = conn.PageInfo.EndCursor
	}
}

// ExplainUnsupportedKind rewrites sdk-go's decoder error for a session or
// entry kind it was not generated for (e.g. a Pipeline session) into one an
// agent can act on. Other errors pass through unchanged.
func ExplainUnsupportedKind(err error, what, id string) error {
	if err == nil || !strings.Contains(err.Error(), "unexpected concrete type") {
		return err
	}
	return fmt.Errorf(
		"%s %s is a kind this tool cannot read (likely part of a Caido "+
			"Pipeline session); caido_list_replay_sessions shows each "+
			"session's kind: %w",
		what, id, err,
	)
}
