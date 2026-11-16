package tools

import (
	"context"
	"fmt"
	"slices"

	gql "github.com/Khan/genqlient/graphql"
	caido "github.com/caido-community/sdk-go"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Caido's updateEnvironment replaces the whole variable list and is guarded
// by an optimistic version, and sdk-go has no operation for it. The tool
// reads the environment, applies set/remove by variable name and writes the
// result back with the version it read.

const (
	maxEnvVariables     = 200
	maxEnvVariableBytes = 64 << 10
)

const getEnvironmentQuery = `query GetEnvironment($id: ID!) {
  environment(id: $id) { id name version variables { name value kind } }
}`

const updateEnvironmentMutation = `mutation UpdateEnvironment($id: ID!, $input: UpdateEnvironmentInput!) {
  updateEnvironment(id: $id, input: $input) {
    environment { id name version variables { name kind } }
    error {
      __typename
      ... on UserError { code }
      ... on PermissionDeniedUserError { reason }
    }
  }
}`

// EnvironmentVariableSet is one variable to create or overwrite.
type EnvironmentVariableSet struct {
	Name  string `json:"name" jsonschema:"required,Variable name"`
	Value string `json:"value" jsonschema:"required,Variable value"`
	Kind  string `json:"kind,omitempty" jsonschema:"PLAIN or SECRET. Omit to keep an existing variable's kind (new variables default to PLAIN)"`
}

// UpdateEnvironmentInput is the input for the update_environment tool.
type UpdateEnvironmentInput struct {
	ID     string                   `json:"id" jsonschema:"required,Environment ID from caido_list_environments"`
	Name   string                   `json:"name,omitempty" jsonschema:"New environment name (omit to keep)"`
	Set    []EnvironmentVariableSet `json:"set,omitempty" jsonschema:"Variables to create or overwrite, matched by name"`
	Remove []string                 `json:"remove,omitempty" jsonschema:"Names of variables to delete"`
}

// EnvironmentVariableRef names a stored variable without its value.
type EnvironmentVariableRef struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// UpdateEnvironmentOutput is the environment after the update. Values are
// left out: the caller just supplied the ones it changed, and echoing the
// rest would put stored secrets into the model's context for no reason.
type UpdateEnvironmentOutput struct {
	ID        string                   `json:"id"`
	Name      string                   `json:"name"`
	Version   int                      `json:"version"`
	Variables []EnvironmentVariableRef `json:"variables"`
}

type envVariable struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Kind  string `json:"kind"`
}

// mergeEnvVariables applies set (upsert by name, keeping position and, when
// no kind is given, the stored kind) and then remove to current.
func mergeEnvVariables(
	current []envVariable, set []EnvironmentVariableSet, remove []string,
) ([]envVariable, error) {
	out := slices.Clone(current)
	for i, v := range set {
		if v.Name == "" {
			return nil, fmt.Errorf("set[%d]: name is required", i)
		}
		if len(v.Value) > maxEnvVariableBytes {
			return nil, fmt.Errorf("set[%d] %q: value exceeds %d bytes", i, v.Name, maxEnvVariableBytes)
		}
		kind := v.Kind
		if kind != "" && kind != "PLAIN" && kind != "SECRET" {
			return nil, fmt.Errorf("set[%d] %q: kind must be PLAIN or SECRET, got %q", i, v.Name, v.Kind)
		}
		if slices.Contains(remove, v.Name) {
			return nil, fmt.Errorf("variable %q is in both set and remove", v.Name)
		}
		j := slices.IndexFunc(out, func(e envVariable) bool { return e.Name == v.Name })
		if j < 0 {
			if kind == "" {
				kind = "PLAIN"
			}
			out = append(out, envVariable{Name: v.Name, Value: v.Value, Kind: kind})
			continue
		}
		out[j].Value = v.Value
		if kind != "" {
			out[j].Kind = kind
		}
	}
	for _, name := range remove {
		j := slices.IndexFunc(out, func(e envVariable) bool { return e.Name == name })
		if j < 0 {
			return nil, fmt.Errorf("remove: no variable named %q", name)
		}
		out = slices.Delete(out, j, j+1)
	}
	if len(out) > maxEnvVariables {
		return nil, fmt.Errorf("environment would hold %d variables, max %d", len(out), maxEnvVariables)
	}
	return out, nil
}

func updateEnvironmentHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, UpdateEnvironmentInput) (*mcp.CallToolResult, UpdateEnvironmentOutput, error) {
	return func(
		ctx context.Context, req *mcp.CallToolRequest, in UpdateEnvironmentInput,
	) (*mcp.CallToolResult, UpdateEnvironmentOutput, error) {
		var none UpdateEnvironmentOutput
		if in.ID == "" {
			return nil, none, fmt.Errorf("id is required")
		}
		if len(in.Name) > 200 {
			return nil, none, fmt.Errorf("name exceeds max length of 200")
		}
		if in.Name == "" && len(in.Set) == 0 && len(in.Remove) == 0 {
			return nil, none, fmt.Errorf("nothing to update: give name, set or remove")
		}

		var current struct {
			Environment *struct {
				Name      string        `json:"name"`
				Version   int           `json:"version"`
				Variables []envVariable `json:"variables"`
			} `json:"environment"`
		}
		if err := client.GraphQL.MakeRequest(ctx, &gql.Request{
			OpName:    "GetEnvironment",
			Query:     getEnvironmentQuery,
			Variables: map[string]any{"id": in.ID},
		}, &gql.Response{Data: &current}); err != nil {
			return nil, none, fmt.Errorf("read environment %s: %w", in.ID, err)
		}
		env := current.Environment
		if env == nil {
			return nil, none, fmt.Errorf("environment %s not found", in.ID)
		}

		merged, err := mergeEnvVariables(env.Variables, in.Set, in.Remove)
		if err != nil {
			return nil, none, err
		}
		name := env.Name
		if in.Name != "" {
			name = in.Name
		}
		// [EnvironmentVariableInput!]! rejects null, so always send a list.
		variables := make([]map[string]any, 0, len(merged))
		for _, v := range merged {
			variables = append(variables, map[string]any{
				"name": v.Name, "value": v.Value, "kind": v.Kind,
			})
		}

		var updated struct {
			UpdateEnvironment struct {
				Environment *struct {
					ID        string                   `json:"id"`
					Name      string                   `json:"name"`
					Version   int                      `json:"version"`
					Variables []EnvironmentVariableRef `json:"variables"`
				} `json:"environment"`
				Error *struct {
					Typename string `json:"__typename"`
					Code     string `json:"code"`
					Reason   string `json:"reason"`
				} `json:"error"`
			} `json:"updateEnvironment"`
		}
		if err := client.GraphQL.MakeRequest(ctx, &gql.Request{
			OpName: "UpdateEnvironment",
			Query:  updateEnvironmentMutation,
			Variables: map[string]any{
				"id": in.ID,
				"input": map[string]any{
					"name": name, "version": env.Version, "variables": variables,
				},
			},
		}, &gql.Response{Data: &updated}); err != nil {
			return nil, none, fmt.Errorf("update environment %s: %w", in.ID, err)
		}
		p := updated.UpdateEnvironment
		if e := p.Error; e != nil {
			msg := e.Typename
			if e.Code != "" {
				msg += ": " + e.Code
			}
			if e.Reason != "" {
				msg += " (" + e.Reason + ")"
			}
			if e.Typename == "NewerVersionUserError" {
				msg += "; the environment changed since it was read, call again"
			}
			return nil, none, fmt.Errorf("update environment %s failed: %s", in.ID, msg)
		}
		if p.Environment == nil {
			return nil, none, fmt.Errorf("update environment %s returned no environment", in.ID)
		}
		out := UpdateEnvironmentOutput{
			ID:        p.Environment.ID,
			Name:      p.Environment.Name,
			Version:   p.Environment.Version,
			Variables: p.Environment.Variables,
		}
		if out.Variables == nil {
			out.Variables = []EnvironmentVariableRef{}
		}
		return nil, out, nil
	}
}

// RegisterUpdateEnvironmentTool registers caido_update_environment.
func RegisterUpdateEnvironmentTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:  "caido_update_environment",
		Title: "Update Environment",
		Description: `Set, overwrite or delete variables in an environment, or rename it. ` +
			`set upserts by variable name (kind PLAIN or SECRET; omitted keeps the stored kind), remove deletes by name; variables not mentioned are kept. ` +
			`Use it to store a token or ID once and reference it from Replay placeholders and workflows. ` +
			`Returns the variable names and kinds, not their values.`,
		InputSchema: schemaFor[UpdateEnvironmentInput](func(s *jsonschema.Schema) {
			if p := prop(s, "name"); p != nil {
				p.MaxLength = intPtr(200)
			}
			if p := prop(s, "set"); p != nil {
				p.MaxItems = intPtr(maxEnvVariables)
				if p.Items != nil {
					if k := prop(p.Items, "kind"); k != nil {
						k.Enum = []any{"PLAIN", "SECRET"}
					}
				}
			}
		}),
		// Overwrites and deletes stored variables.
		Annotations: writeAnn(true, true, false),
	}, updateEnvironmentHandler(client))
}
