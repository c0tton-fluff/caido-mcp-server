package tools_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/testutil"
	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/tools"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// These tests run against a mock with schema validation enabled, so a
// section/operation shape the real Caido server would reject fails here
// too. Verified against a live Caido 0.57.0 before being encoded:
// requestMethod, requestSNI and responseStatusCode each rejected the
// `matcher` field their input types do not declare, and requestHeader
// accepted all four operation modes.

// createOKData is the CreateTamperRule payload for a successful create.
func createOKData() map[string]any {
	return map[string]any{
		"createTamperRule": map[string]any{
			"error": nil,
			"rule":  map[string]any{"id": "rule-1", "name": "r"},
		},
	}
}

// newValidatingCreateEnv builds an MCP env whose mock validates every
// GraphQL call against the pinned Caido schema.
func newValidatingCreateEnv(t *testing.T) *testutil.MCPTestEnv {
	t.Helper()
	env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
		tools.RegisterCreateTamperRuleTool(s, c)
	})
	env.Mock.ValidateAgainstSchema()
	env.Mock.On("CreateTamperRule", createOKData())
	return env
}

// sectionVars pulls the section object out of the recorded GraphQL
// variables for the CreateTamperRule operation.
func sectionVars(t *testing.T, env *testutil.MCPTestEnv) map[string]any {
	t.Helper()
	vars := env.Mock.LastVariables("CreateTamperRule")
	if vars == nil {
		t.Fatal("CreateTamperRule was never called")
	}
	input, ok := vars["input"].(map[string]any)
	if !ok {
		t.Fatalf("input is not an object: %#v", vars["input"])
	}
	section, ok := input["section"].(map[string]any)
	if !ok {
		t.Fatalf("section is not an object: %#v", input["section"])
	}
	return section
}

// operationVars descends section -> <sectionKey> -> operation and returns
// the single operation variant key and its body.
func operationVars(
	t *testing.T, section map[string]any, sectionKey string,
) (string, map[string]any) {
	t.Helper()
	sec, ok := section[sectionKey].(map[string]any)
	if !ok {
		t.Fatalf("section %q missing, got keys %v", sectionKey, keysOf(section))
	}
	op, ok := sec["operation"].(map[string]any)
	if !ok {
		t.Fatalf("operation missing under %q", sectionKey)
	}
	if len(op) != 1 {
		t.Fatalf("operation must be exactly one oneof variant, got %v", keysOf(op))
	}
	for k, v := range op {
		body, ok := v.(map[string]any)
		if !ok {
			t.Fatalf("operation %q body is not an object: %#v", k, v)
		}
		return k, body
	}
	return "", nil
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestTamperSectionsWithoutMatcher is the regression test for the bug this
// table refactor fixed. requestMethod, responseStatusCode and requestSNI
// declare a replacer and NO matcher; the previous builder sent a matcher
// for all three, and a live server rejected the call. This test fails
// against that builder and passes now, which is the only reason it earns
// its place.
func TestTamperSectionsWithoutMatcher(t *testing.T) {
	tests := []struct {
		section    string
		sectionKey string
		wantOp     string
	}{
		{"requestMethod", "requestMethod", "update"},
		{"responseStatusCode", "responseStatusCode", "update"},
		{"requestSNI", "requestSNI", "raw"},
	}

	for _, tt := range tests {
		t.Run(tt.section, func(t *testing.T) {
			env := newValidatingCreateEnv(t)

			result := env.CallTool(t, "caido_create_tamper_rule", map[string]any{
				"collection_id": "col-1",
				"name":          "r",
				"section":       tt.section,
				"replace":       "POST",
			})
			if result.IsError {
				t.Fatalf("unexpected error: %s", contentText(result))
			}

			opKey, body := operationVars(
				t, sectionVars(t, env), tt.sectionKey,
			)
			if opKey != tt.wantOp {
				t.Fatalf("operation key = %q, want %q", opKey, tt.wantOp)
			}
			if _, bad := body["matcher"]; bad {
				t.Fatalf(
					"section %q must not send a matcher; body = %v",
					tt.section, keysOf(body),
				)
			}
			if _, ok := body["replacer"]; !ok {
				t.Fatalf("section %q must send a replacer", tt.section)
			}
		})
	}
}

// TestTamperTableIsSchemaValid walks EVERY section and EVERY operation
// mode the table declares and pushes each one through the create tool
// against the schema-validating mock.
//
// The hand-picked cases elsewhere in this file prove specific shapes; this
// proves there is no entry in the table that disagrees with the Caido
// schema. It is the test that would catch a future section added with the
// wrong matcher kind, a wrong gqlKey, or a replacer that should not be
// there -- none of which the sampled tests would notice.
//
// tools.TamperSectionModes exposes the table for exactly this purpose.
func TestTamperTableIsSchemaValid(t *testing.T) {
	modes := tools.TamperSectionModes()
	if len(modes) == 0 {
		t.Fatal("section table is empty")
	}

	combos := 0
	for _, section := range sortedKeys(modes) {
		for _, kind := range modes[section] {
			combos++
			t.Run(section+"/"+kind, func(t *testing.T) {
				env := newValidatingCreateEnv(t)

				result := env.CallTool(
					t, "caido_create_tamper_rule",
					map[string]any{
						"collection_id": "col-1",
						"name":          "r",
						"section":       section,
						"operation": minimalOperation(
							t, section, kind,
						),
					},
				)
				if result.IsError {
					t.Fatalf(
						"section %q mode %q rejected: %s",
						section, kind, contentText(result),
					)
				}

				// The call reaching the mock at all means it passed schema
				// validation. Also confirm exactly one oneof variant was
				// sent at the operation level.
				operationVars(
					t, sectionVars(t, env),
					tools.TamperSectionGQLKey(section),
				)
			})
		}
	}

	// Guard against the table silently shrinking: 7 raw-only sections,
	// 3 named-field sections x 4 modes, 3 value-only sections.
	if want := 7 + 12 + 3; combos != want {
		t.Fatalf(
			"table covers %d section/mode combinations, want %d",
			combos, want,
		)
	}
}

// minimalOperation builds the smallest valid operation payload for a
// section/mode pair, deriving the required fields from what the mode
// accepts rather than from a hardcoded list.
func minimalOperation(
	t *testing.T, section, kind string,
) map[string]any {
	t.Helper()
	op := map[string]any{"kind": kind}

	switch tools.TamperMatcherKindFor(section, kind) {
	case "raw":
		op["match"] = "x"
	case "name":
		op["name"] = "X-Test"
	case "none":
		// Applies unconditionally; no matcher fields.
	default:
		t.Fatalf("unknown matcher kind for %s/%s", section, kind)
	}

	if tools.TamperHasReplacer(section, kind) {
		op["value"] = "v"
	}
	return op
}

func sortedKeys(m map[string][]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestTamperHeaderOperationModes covers the four modes on a named-field
// section, asserting the exact matcher/replacer shape each one emits.
func TestTamperHeaderOperationModes(t *testing.T) {
	tests := []struct {
		name         string
		operation    map[string]any
		wantOp       string
		wantMatcher  map[string]any
		wantReplacer map[string]any
	}{
		{
			name: "updateValue uses a name matcher",
			operation: map[string]any{
				"kind":  "updateValue",
				"name":  "X-Request-ID",
				"value": "fixed",
			},
			wantOp:       "update",
			wantMatcher:  map[string]any{"name": "X-Request-ID"},
			wantReplacer: map[string]any{"term": map[string]any{"term": "fixed"}},
		},
		{
			name: "add uses a name matcher",
			operation: map[string]any{
				"kind":  "add",
				"name":  "X-New",
				"value": "v1",
			},
			wantOp:       "add",
			wantMatcher:  map[string]any{"name": "X-New"},
			wantReplacer: map[string]any{"term": map[string]any{"term": "v1"}},
		},
		{
			name: "remove sends no replacer",
			operation: map[string]any{
				"kind": "remove",
				"name": "X-Request-ID",
			},
			wantOp:       "remove",
			wantMatcher:  map[string]any{"name": "X-Request-ID"},
			wantReplacer: nil,
		},
		{
			name: "updateRaw with a literal value matcher",
			operation: map[string]any{
				"kind":       "updateRaw",
				"match":      "abc",
				"match_kind": "value",
				"value":      "zzz",
			},
			wantOp:       "raw",
			wantMatcher:  map[string]any{"value": map[string]any{"value": "abc"}},
			wantReplacer: map[string]any{"term": map[string]any{"term": "zzz"}},
		},
		{
			name: "updateRaw full matcher takes no match",
			operation: map[string]any{
				"kind":       "updateRaw",
				"match_kind": "full",
				"value":      "X",
			},
			wantOp:       "raw",
			wantMatcher:  map[string]any{"full": map[string]any{"full": true}},
			wantReplacer: map[string]any{"term": map[string]any{"term": "X"}},
		},
		{
			name: "workflow replacer instead of a term",
			operation: map[string]any{
				"kind":        "updateValue",
				"name":        "X-Sig",
				"workflow_id": "wf-1",
			},
			wantOp:       "update",
			wantMatcher:  map[string]any{"name": "X-Sig"},
			wantReplacer: map[string]any{"workflow": map[string]any{"id": "wf-1"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newValidatingCreateEnv(t)

			result := env.CallTool(t, "caido_create_tamper_rule", map[string]any{
				"collection_id": "col-1",
				"name":          "r",
				"section":       "requestHeader",
				"operation":     tt.operation,
			})
			if result.IsError {
				t.Fatalf("unexpected error: %s", contentText(result))
			}

			opKey, body := operationVars(
				t, sectionVars(t, env), "requestHeader",
			)
			if opKey != tt.wantOp {
				t.Fatalf("operation key = %q, want %q", opKey, tt.wantOp)
			}
			assertJSONEqual(t, "matcher", body["matcher"], tt.wantMatcher)
			assertJSONEqual(t, "replacer", body["replacer"], tt.wantReplacer)
		})
	}
}

// TestTamperUpdateToolOperationModes proves the update tool's own wiring,
// not just the shared builder. It calls resolveTamperOperation separately
// from create, so a create-only test would leave this path unproven.
func TestTamperUpdateToolOperationModes(t *testing.T) {
	okData := map[string]any{
		"updateTamperRule": map[string]any{
			"error": nil,
			"rule":  map[string]any{"id": "rule-1", "name": "r"},
		},
	}

	tests := []struct {
		name      string
		args      map[string]any
		wantOp    string
		wantNoKey string
	}{
		{
			name: "updateValue via operation",
			args: map[string]any{
				"section": "responseHeader",
				"operation": map[string]any{
					"kind": "updateValue", "name": "X-Frame-Options",
					"value": "DENY",
				},
			},
			wantOp: "update",
		},
		{
			name: "remove via operation sends no replacer",
			args: map[string]any{
				"section": "requestHeader",
				"operation": map[string]any{
					"kind": "remove", "name": "Cookie",
				},
			},
			wantOp:    "remove",
			wantNoKey: "replacer",
		},
		{
			name: "matcherless section via operation",
			args: map[string]any{
				"section": "responseStatusCode",
				"operation": map[string]any{
					"kind": "updateValue", "value": "404",
				},
			},
			wantOp:    "update",
			wantNoKey: "matcher",
		},
		{
			name: "legacy match/replace still works",
			args: map[string]any{
				"section": "requestHeader",
				"match":   "X-Foo", "replace": "X-Bar",
			},
			wantOp: "raw",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterUpdateTamperRuleTool(s, c)
			})
			env.Mock.ValidateAgainstSchema()
			env.Mock.On("UpdateTamperRule", okData)

			args := map[string]any{"id": "rule-1", "name": "r"}
			for k, v := range tt.args {
				args[k] = v
			}

			result := env.CallTool(t, "caido_update_tamper_rule", args)
			if result.IsError {
				t.Fatalf("unexpected error: %s", contentText(result))
			}

			vars := env.Mock.LastVariables("UpdateTamperRule")
			if vars == nil {
				t.Fatal("UpdateTamperRule was never called")
			}
			input, _ := vars["input"].(map[string]any)
			section, ok := input["section"].(map[string]any)
			if !ok {
				t.Fatalf("section missing from update input: %#v", input)
			}
			sectionKey := tools.TamperSectionGQLKey(
				tt.args["section"].(string),
			)
			opKey, body := operationVars(t, section, sectionKey)
			if opKey != tt.wantOp {
				t.Fatalf("operation key = %q, want %q", opKey, tt.wantOp)
			}
			if tt.wantNoKey != "" {
				if _, bad := body[tt.wantNoKey]; bad {
					t.Fatalf(
						"%s must not be sent for %s; body = %v",
						tt.wantNoKey, tt.name, keysOf(body),
					)
				}
			}
		})
	}
}

// TestTamperSectionFromTypename covers the __typename mapping used by the
// listing.
//
// Only two outcomes are reachable in production. A __typename outside the
// TamperSection union cannot get here at all: genqlient's generated
// unmarshaller rejects the whole response first (confirmed by feeding it
// "SomethingElse", which fails with "unexpected concrete type"). So the
// cases that matter are a section the tools expose and a valid union
// member they do not.
func TestTamperSectionFromTypename(t *testing.T) {
	// Drive it through the listing tool so the mapping is exercised the
	// way production calls it.
	tests := []struct {
		name     string
		typename any
		want     string
	}{
		{"exposed section", "TamperSectionRequestHeader", "requestHeader"},
		{"exposed response section", "TamperSectionResponseStatusCode", "responseStatusCode"},
		{"valid union member the tools do not expose", "TamperSectionStreamWsMessageDownstream", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterListTamperRulesTool(s, c)
			})
			env.Mock.On("ListTamperRuleCollections", map[string]any{
				"tamperRuleCollections": []map[string]any{{
					"id": "col-1", "name": "c",
					"rules": []map[string]any{{
						"id": "rule-1", "name": "r",
						"sources": []string{},
						"section": map[string]any{
							"__typename": tt.typename,
						},
					}},
				}},
			})

			result := env.CallTool(
				t, "caido_list_tamper_rules", map[string]any{},
			)
			if result.IsError {
				t.Fatalf("unexpected error: %s", contentText(result))
			}
			out := testutil.UnmarshalToolResult[tools.ListTamperRulesOutput](
				t, result,
			)
			got := out.Collections[0].Rules[0].Section
			if got != tt.want {
				t.Fatalf("section = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestTamperLegacyMatchReplace pins the backwards-compatible path: no
// operation object means updateRaw with a regex matcher, which is exactly
// what pre-existing callers sent.
func TestTamperLegacyMatchReplace(t *testing.T) {
	env := newValidatingCreateEnv(t)

	result := env.CallTool(t, "caido_create_tamper_rule", map[string]any{
		"collection_id": "col-1",
		"name":          "r",
		"section":       "requestHeader",
		"match":         "X-Foo",
		"replace":       "X-Bar",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", contentText(result))
	}

	opKey, body := operationVars(t, sectionVars(t, env), "requestHeader")
	if opKey != "raw" {
		t.Fatalf("legacy path must build a raw operation, got %q", opKey)
	}
	assertJSONEqual(t, "matcher", body["matcher"],
		map[string]any{"regex": map[string]any{"regex": "X-Foo"}})
	assertJSONEqual(t, "replacer", body["replacer"],
		map[string]any{"term": map[string]any{"term": "X-Bar"}})
}

// TestTamperOperationRejections covers the validation that turns a remote
// coercion failure into a local, actionable message.
func TestTamperOperationRejections(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
	}{
		{
			name: "mode not supported by section",
			args: map[string]any{
				"section": "requestBody",
				"operation": map[string]any{
					"kind": "remove", "name": "X-Foo",
				},
			},
		},
		{
			name: "updateValue without a name",
			args: map[string]any{
				"section": "requestHeader",
				"operation": map[string]any{
					"kind": "updateValue", "value": "v",
				},
			},
		},
		{
			name: "remove with a value",
			args: map[string]any{
				"section": "requestHeader",
				"operation": map[string]any{
					"kind": "remove", "name": "X-Foo", "value": "v",
				},
			},
		},
		{
			name: "name given to a matcherless section",
			args: map[string]any{
				"section": "requestMethod",
				"operation": map[string]any{
					"kind": "updateValue", "name": "X-Foo", "value": "POST",
				},
			},
		},
		{
			name: "value and workflow_id together",
			args: map[string]any{
				"section": "requestHeader",
				"operation": map[string]any{
					"kind": "updateValue", "name": "X-Foo",
					"value": "v", "workflow_id": "wf-1",
				},
			},
		},
		{
			name: "unknown match_kind",
			args: map[string]any{
				"section": "requestBody",
				"operation": map[string]any{
					"kind": "updateRaw", "match": "a", "match_kind": "glob",
				},
			},
		},
		{
			name: "full match_kind with a match",
			args: map[string]any{
				"section": "requestBody",
				"operation": map[string]any{
					"kind": "updateRaw", "match": "a", "match_kind": "full",
				},
			},
		},
		{
			name: "unknown section",
			args: map[string]any{"section": "notASection"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newValidatingCreateEnv(t)

			args := map[string]any{"collection_id": "col-1", "name": "r"}
			for k, v := range tt.args {
				args[k] = v
			}

			result := env.CallTool(t, "caido_create_tamper_rule", args)
			if !result.IsError {
				t.Fatalf(
					"expected a local validation error, got success; vars=%v",
					env.Mock.LastVariables("CreateTamperRule"),
				)
			}
		})
	}
}

// TestTestTamperRuleTool covers the dry-run tool, including the changed
// flag that tells an agent its rule matched nothing.
func TestTestTamperRuleTool(t *testing.T) {
	const input = "GET / HTTP/1.1\r\nHost: a.com\r\nX-Request-ID: abc\r\n\r\n"
	const transformed = "GET / HTTP/1.1\r\nHost: a.com\r\nX-Request-ID: fixed\r\n\r\n"

	tests := []struct {
		name        string
		raw         string
		respRaw     string
		wantChanged bool
	}{
		{
			name:        "reports a transformation",
			raw:         input,
			respRaw:     transformed,
			wantChanged: true,
		},
		{
			name:        "reports no match when output is unchanged",
			raw:         input,
			respRaw:     input,
			wantChanged: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
				tools.RegisterTestTamperRuleTool(s, c)
			})
			env.Mock.ValidateAgainstSchema()
			env.Mock.On("TestTamperRule", map[string]any{
				"testTamperRule": map[string]any{
					"error": nil,
					"raw": base64.StdEncoding.EncodeToString(
						[]byte(tt.respRaw),
					),
				},
			})

			result := env.CallTool(t, "caido_test_tamper_rule", map[string]any{
				"raw":     tt.raw,
				"section": "requestHeader",
				"operation": map[string]any{
					"kind": "updateValue", "name": "X-Request-ID",
					"value": "fixed",
				},
			})
			if result.IsError {
				t.Fatalf("unexpected error: %s", contentText(result))
			}

			out := testutil.UnmarshalToolResult[tools.TestTamperRuleOutput](
				t, result,
			)
			if out.Raw != tt.respRaw {
				t.Fatalf("raw = %q, want %q", out.Raw, tt.respRaw)
			}
			if out.Changed != tt.wantChanged {
				t.Fatalf("changed = %v, want %v", out.Changed, tt.wantChanged)
			}

			// The request must carry base64, not plain text: raw is a Blob.
			vars := env.Mock.LastVariables("TestTamperRule")
			gqlInput, _ := vars["input"].(map[string]any)
			wantRaw := base64.StdEncoding.EncodeToString([]byte(tt.raw))
			if got, _ := gqlInput["raw"].(string); got != wantRaw {
				t.Fatalf("raw variable = %q, want base64 %q", got, wantRaw)
			}
		})
	}
}

// TestMockSchemaValidationRejectsUnknownField is the negative control for
// the validating mock itself. A mock that silently accepts everything
// looks identical to one that validates, so prove it can reject: this
// sends the exact payload the old builder produced for requestMethod and
// requires the mock to refuse it.
func TestMockSchemaValidationRejectsUnknownField(t *testing.T) {
	env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
		tools.RegisterCreateTamperRuleTool(s, c)
	})
	env.Mock.ValidateAgainstSchema()
	env.Mock.On("CreateTamperRule", createOKData())

	// Bypass the tool layer and post the legacy shape straight at the mock.
	body := map[string]any{
		"operationName": "CreateTamperRule",
		"query": "mutation CreateTamperRule($input: CreateTamperRuleInput!) " +
			"{ createTamperRule(input: $input) { rule { id } } }",
		"variables": map[string]any{
			"input": map[string]any{
				"collectionId": "col-1",
				"name":         "r",
				"sources":      []string{},
				"section": map[string]any{
					"requestMethod": map[string]any{
						"operation": map[string]any{
							"update": map[string]any{
								"matcher": map[string]any{
									"regex": map[string]any{"regex": "GET"},
								},
								"replacer": map[string]any{
									"term": map[string]any{"term": "POST"},
								},
							},
						},
					},
				},
			},
		},
	}
	raw := postJSON(t, env.Server.URL, body)
	if !containsAll(raw, "schema validation", "matcher") {
		t.Fatalf(
			"validating mock accepted the known-bad payload; response = %s",
			raw,
		)
	}
}

// postJSON posts body to url and returns the response as a string.
func postJSON(t *testing.T, url string, body map[string]any) string {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	resp, err := http.Post(
		url, "application/json", bytes.NewReader(encoded),
	)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	return string(out)
}

// contentText renders a tool result's content as readable text. Printing
// the Content slice directly yields pointer addresses, which is useless
// in a failure message.
func contentText(result *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range result.Content {
		if text, ok := c.(*mcp.TextContent); ok {
			b.WriteString(text.Text)
			continue
		}
		fmt.Fprintf(&b, "%T", c)
	}
	return b.String()
}

// containsAll reports whether s contains every one of subs.
func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

func assertJSONEqual(t *testing.T, label string, got, want any) {
	t.Helper()
	gotJSON, _ := json.Marshal(got)
	wantJSON, _ := json.Marshal(want)
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("%s = %s, want %s", label, gotJSON, wantJSON)
	}
}
