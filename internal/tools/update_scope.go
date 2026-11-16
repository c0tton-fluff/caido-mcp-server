package tools

import (
	"context"
	"fmt"

	gql "github.com/Khan/genqlient/graphql"
	caido "github.com/caido-community/sdk-go"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Caido's updateScope replaces name, allowlist and denylist together, and
// sdk-go has no operation for it (only renameScope). The tool reads the
// scope first so a caller can change one list without restating the rest.

const maxScopeEntries = 100

const getScopeQuery = `query GetScope($id: ID!) {
  scope(id: $id) { id name allowlist denylist }
}`

const updateScopeMutation = `mutation UpdateScope($id: ID!, $input: UpdateScopeInput!) {
  updateScope(id: $id, input: $input) {
    scope { id name allowlist denylist }
    error { __typename ... on UserError { code } }
  }
}`

// UpdateScopeInput is the input for the update_scope tool. A nil list means
// "leave as is"; an empty denylist clears it.
type UpdateScopeInput struct {
	ID        string   `json:"id" jsonschema:"required,Scope ID from caido_list_scopes"`
	Name      string   `json:"name,omitempty" jsonschema:"New name (omit to keep)"`
	Allowlist []string `json:"allowlist,omitempty" jsonschema:"Replace the allowlist with these host patterns (omit to keep)"`
	Denylist  []string `json:"denylist,omitempty" jsonschema:"Replace the denylist with these host patterns; [] clears it (omit to keep)"`
	AddAllow  []string `json:"addAllow,omitempty" jsonschema:"Host patterns to append to the allowlist, duplicates skipped"`
	AddDeny   []string `json:"addDeny,omitempty" jsonschema:"Host patterns to append to the denylist, duplicates skipped"`
}

// UpdateScopeOutput is the scope as stored after the update.
type UpdateScopeOutput struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Allowlist []string `json:"allowlist"`
	Denylist  []string `json:"denylist"`
}

type scopeNode struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Allowlist []string `json:"allowlist"`
	Denylist  []string `json:"denylist"`
}

// appendUnique appends the items of add that base does not already hold,
// keeping order. The result is never nil, so it marshals to [] rather than
// the null a [String!]! field rejects.
func appendUnique(base, add []string) []string {
	out := make([]string, 0, len(base)+len(add))
	seen := make(map[string]bool, len(base)+len(add))
	for _, list := range [][]string{base, add} {
		for _, v := range list {
			if !seen[v] {
				seen[v] = true
				out = append(out, v)
			}
		}
	}
	return out
}

func updateScopeHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, UpdateScopeInput) (*mcp.CallToolResult, UpdateScopeOutput, error) {
	return func(
		ctx context.Context, req *mcp.CallToolRequest, in UpdateScopeInput,
	) (*mcp.CallToolResult, UpdateScopeOutput, error) {
		var none UpdateScopeOutput
		if in.ID == "" {
			return nil, none, fmt.Errorf("id is required")
		}
		if len(in.Name) > 200 {
			return nil, none, fmt.Errorf("name exceeds max length of 200")
		}
		if in.Name == "" && in.Allowlist == nil && in.Denylist == nil &&
			len(in.AddAllow) == 0 && len(in.AddDeny) == 0 {
			return nil, none, fmt.Errorf(
				"nothing to update: give name, allowlist, denylist, addAllow or addDeny",
			)
		}

		var current struct {
			Scope *scopeNode `json:"scope"`
		}
		if err := client.GraphQL.MakeRequest(ctx, &gql.Request{
			OpName:    "GetScope",
			Query:     getScopeQuery,
			Variables: map[string]any{"id": in.ID},
		}, &gql.Response{Data: &current}); err != nil {
			return nil, none, fmt.Errorf("read scope %s: %w", in.ID, err)
		}
		if current.Scope == nil {
			return nil, none, fmt.Errorf("scope %s not found", in.ID)
		}

		name := current.Scope.Name
		if in.Name != "" {
			name = in.Name
		}
		allow := current.Scope.Allowlist
		if in.Allowlist != nil {
			allow = in.Allowlist
		}
		deny := current.Scope.Denylist
		if in.Denylist != nil {
			deny = in.Denylist
		}
		allow = appendUnique(allow, in.AddAllow)
		deny = appendUnique(deny, in.AddDeny)
		if len(allow) == 0 {
			return nil, none, fmt.Errorf("allowlist would be empty; a scope needs at least one entry")
		}
		if len(allow) > maxScopeEntries || len(deny) > maxScopeEntries {
			return nil, none, fmt.Errorf(
				"allowlist and denylist are limited to %d entries each", maxScopeEntries,
			)
		}

		var updated struct {
			UpdateScope struct {
				Scope *scopeNode `json:"scope"`
				Error *struct {
					Typename string `json:"__typename"`
					Code     string `json:"code"`
				} `json:"error"`
			} `json:"updateScope"`
		}
		if err := client.GraphQL.MakeRequest(ctx, &gql.Request{
			OpName: "UpdateScope",
			Query:  updateScopeMutation,
			Variables: map[string]any{
				"id": in.ID,
				"input": map[string]any{
					"name": name, "allowlist": allow, "denylist": deny,
				},
			},
		}, &gql.Response{Data: &updated}); err != nil {
			return nil, none, fmt.Errorf("update scope %s: %w", in.ID, err)
		}
		p := updated.UpdateScope
		if e := p.Error; e != nil {
			msg := e.Typename
			if e.Code != "" {
				msg += ": " + e.Code
			}
			if e.Typename == "InvalidGlobTermsUserError" {
				msg += "; entries are host patterns such as example.com or *.example.com, without scheme or path"
			}
			return nil, none, fmt.Errorf("update scope %s failed: %s", in.ID, msg)
		}
		if p.Scope == nil {
			return nil, none, fmt.Errorf("update scope %s returned no scope", in.ID)
		}
		return nil, UpdateScopeOutput{
			ID:        p.Scope.ID,
			Name:      p.Scope.Name,
			Allowlist: appendUnique(p.Scope.Allowlist, nil),
			Denylist:  appendUnique(p.Scope.Denylist, nil),
		}, nil
	}
}

// RegisterUpdateScopeTool registers caido_update_scope.
func RegisterUpdateScopeTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:  "caido_update_scope",
		Title: "Update Scope",
		Description: `Change an existing scope's name, allowlist or denylist. ` +
			`addAllow / addDeny append host patterns without restating the lists; allowlist / denylist replace them (denylist [] clears it). ` +
			`Omitted fields keep their current value. Entries are host patterns ("example.com", "*.example.com"), no scheme or path. ` +
			`Returns the scope as stored.`,
		InputSchema: schemaFor[UpdateScopeInput](func(s *jsonschema.Schema) {
			if p := prop(s, "name"); p != nil {
				p.MaxLength = intPtr(200)
			}
			for _, f := range []string{"allowlist", "denylist", "addAllow", "addDeny"} {
				if p := prop(s, f); p != nil {
					p.MaxItems = intPtr(maxScopeEntries)
				}
			}
		}),
		// Replacing a list overwrites what was stored.
		Annotations: writeAnn(true, true, false),
	}, updateScopeHandler(client))
}
