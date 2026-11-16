package tools_test

import (
	"context"
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/replay"
	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/testutil"
	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/tools"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// listWith registers the server with opts and returns what a client sees:
// the listed tools, the count RegisterAllWith reported, and its error.
func listWith(t *testing.T, opts tools.Options) ([]*mcp.Tool, int, error) {
	t.Helper()
	t.Cleanup(func() { replay.SetTargetGuard(nil) })
	var reported int
	var regErr error
	env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
		reported, regErr = tools.RegisterAllWith(context.Background(), s, c, opts)
	})
	if regErr != nil {
		return nil, 0, regErr
	}
	listed, err := env.MCPClient.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	return listed.Tools, reported, nil
}

func toolNames(list []*mcp.Tool) []string {
	names := make([]string, len(list))
	for i, tool := range list {
		names[i] = tool.Name
	}
	slices.Sort(names)
	return names
}

// TestToolsetSelection pins what each selection registers. The expected
// names are literals on purpose: deriving them from the group table would
// make the assertion agree with whatever the table says.
func TestToolsetSelection(t *testing.T) {
	history := []string{
		"caido_convert_body", "caido_diff_responses", "caido_get_request",
		"caido_get_sitemap", "caido_list_requests",
	}
	findings := []string{
		"caido_create_finding", "caido_delete_findings", "caido_export_findings",
		"caido_list_findings",
	}
	tests := []struct {
		name     string
		toolsets []string
		want     []string
	}{
		{"one toolset", []string{"history"}, history},
		{"two toolsets, order and case do not matter", []string{" Findings", "HISTORY"}, slices.Concat(history, findings)},
		{"duplicates collapse", []string{"history", "history"}, history},
		{"race", []string{"race"}, []string{"caido_race_pipeline", "caido_race_window_send"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list, reported, err := listWith(t, tools.Options{Toolsets: tt.toolsets})
			if err != nil {
				t.Fatalf("RegisterAllWith: %v", err)
			}
			want := slices.Clone(tt.want)
			slices.Sort(want)
			if got := toolNames(list); !slices.Equal(got, want) {
				t.Fatalf("registered %v, want %v", got, want)
			}
			if reported != len(list) {
				t.Errorf("reported %d tools, %d listed", reported, len(list))
			}
		})
	}
}

// TestToolsetsPartitionEveryTool: selecting every toolset by name gives
// exactly the default server, so no tool is outside a toolset or in two.
func TestToolsetsPartitionEveryTool(t *testing.T) {
	all, allCount, err := listWith(t, tools.Options{})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	sum := 0
	for _, line := range tools.ToolsetNames() {
		name, _, _ := strings.Cut(line, " - ")
		if strings.Contains(line, "shorthand for") {
			continue
		}
		names = append(names, name)
		one, _, err := listWith(t, tools.Options{Toolsets: []string{name}})
		if err != nil {
			t.Fatalf("toolset %q: %v", name, err)
		}
		if len(one) == 0 {
			t.Errorf("toolset %q is empty", name)
		}
		sum += len(one)
	}
	if sum != len(all) || allCount != len(all) {
		t.Fatalf("toolsets hold %d tools in total, the default server lists %d (reported %d)", sum, len(all), allCount)
	}
	every, _, err := listWith(t, tools.Options{Toolsets: names})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(toolNames(every), toolNames(all)) {
		t.Fatal("every toolset together is not the default server")
	}
}

func TestToolsetErrors(t *testing.T) {
	tests := []struct {
		name     string
		toolsets []string
		want     string
	}{
		{"unknown name lists the valid ones", []string{"history", "histroy"}, `unknown toolset "histroy" (valid: history, replay,`},
		{"only blanks", []string{" ", ""}, "names no toolset"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := listWith(t, tools.Options{Toolsets: tt.toolsets})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

// TestToolsetsCombineWithRestrictions: the reported count stays the listed
// count when a toolset selection meets read-only mode or scope enforcement,
// and each restriction still holds inside the selection.
func TestToolsetsCombineWithRestrictions(t *testing.T) {
	t.Run("read-only inside a selection", func(t *testing.T) {
		list, reported, err := listWith(t, tools.Options{Toolsets: []string{"findings", "scopes"}, ReadOnly: true})
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"caido_export_findings", "caido_is_in_scope", "caido_list_findings", "caido_list_scopes"}
		if got := toolNames(list); !slices.Equal(got, want) || reported != len(list) {
			t.Fatalf("registered %v (reported %d), want %v", got, reported, want)
		}
	})
	t.Run("scope lock inside a selection", func(t *testing.T) {
		list, reported, err := listWith(t, tools.Options{Toolsets: []string{"scopes"}, EnforceScope: true})
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"caido_is_in_scope", "caido_list_scopes", "caido_rename_scope"}
		if got := toolNames(list); !slices.Equal(got, want) || reported != len(list) {
			t.Fatalf("registered %v (reported %d), want %v", got, reported, want)
		}
	})
	t.Run("scope lock with no locked tool selected", func(t *testing.T) {
		list, reported, err := listWith(t, tools.Options{Toolsets: []string{"history"}, EnforceScope: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 5 || reported != 5 {
			t.Fatalf("listed %d, reported %d, want 5", len(list), reported)
		}
	})
}

// TestCompactDropsOutputSchemas: compact removes output schemas from the
// advertised list and nothing else, and a tool still answers with
// structured content.
func TestCompactDropsOutputSchemas(t *testing.T) {
	full, _, err := listWith(t, tools.Options{})
	if err != nil {
		t.Fatal(err)
	}
	// By default every tool with a typed result advertises its output
	// schema. caido_get_request is the one exception (its result shape
	// depends on the include parameter). Any other tool without one means
	// schemas are being dropped without compact being asked for.
	for _, tool := range full {
		if tool.OutputSchema == nil && tool.Name != "caido_get_request" {
			t.Fatalf("%s advertises no output schema by default", tool.Name)
		}
	}

	t.Cleanup(func() { replay.SetTargetGuard(nil) })
	env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
		if _, err := tools.RegisterAllWith(context.Background(), s, c, tools.Options{Compact: true}); err != nil {
			t.Fatalf("RegisterAllWith: %v", err)
		}
	})
	listed, err := env.MCPClient.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Tools) != len(full) {
		t.Fatalf("compact lists %d tools, default lists %d", len(listed.Tools), len(full))
	}
	for i, tool := range listed.Tools {
		if tool.OutputSchema != nil {
			t.Errorf("%s still advertises an output schema in compact mode", tool.Name)
		}
		if tool.InputSchema == nil || tool.Description != full[i].Description || tool.Annotations == nil {
			t.Errorf("%s lost more than its output schema", tool.Name)
		}
	}

	env.Mock.On("ListScopes", map[string]any{"scopes": []map[string]any{}})
	result := env.CallTool(t, "caido_list_scopes", map[string]any{})
	if result.IsError || result.StructuredContent == nil {
		t.Fatalf("compact mode must still return structured content, got %+v", result)
	}
}

// toolListBytes is the size of the advertised tool list as JSON.
func toolListBytes(t *testing.T, list []*mcp.Tool) int {
	t.Helper()
	b, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	return len(b)
}

// TestToolListBudget keeps the context cost of the tool list from creeping
// up unnoticed. Every session pays these bytes before the first call. The
// budgets sit a little above today's sizes; raise one deliberately when a
// new tool earns its place, not by accident.
func TestToolListBudget(t *testing.T) {
	tests := []struct {
		name   string
		opts   tools.Options
		budget int
	}{
		{"default", tools.Options{}, budgetDefault},
		{"compact", tools.Options{Compact: true}, budgetCompact},
		{"core", tools.Options{Toolsets: []string{"core"}}, budgetCore},
		{"core compact", tools.Options{Toolsets: []string{"core"}, Compact: true}, budgetCoreCompact},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list, _, err := listWith(t, tt.opts)
			if err != nil {
				t.Fatal(err)
			}
			n := toolListBytes(t, list)
			t.Logf("%s: %d tools, %d bytes (budget %d)", tt.name, len(list), n, tt.budget)
			if n > tt.budget {
				t.Errorf("%s tool list is %d bytes, over the %d byte budget", tt.name, n, tt.budget)
			}
		})
	}
}

// TestInstructionsFor: restricted servers get instructions that name no
// tool they might not have.
func TestInstructionsFor(t *testing.T) {
	named := regexp.MustCompile(`caido_[a-z_]+`)
	if got := tools.InstructionsFor(tools.Options{}); got != tools.Instructions {
		t.Error("default options must use the full instructions")
	}
	if got := tools.InstructionsFor(tools.Options{ReadOnly: true, EnforceScope: true}); got != tools.InstructionsReadOnly {
		t.Error("read-only must use the read-only instructions, without the scope note")
	}
	if got := tools.InstructionsFor(tools.Options{EnforceScope: true}); !strings.HasPrefix(got, tools.Instructions) || !strings.Contains(got, "Scope is enforced") {
		t.Error("scope enforcement must append its note to the full instructions")
	}
	for _, opts := range []tools.Options{
		{Toolsets: []string{"history"}},
		{Toolsets: []string{"history"}, ReadOnly: true},
		{Toolsets: []string{"replay"}, EnforceScope: true},
	} {
		got := tools.InstructionsFor(opts)
		if tool := named.FindString(got); tool != "" {
			t.Errorf("instructions for %+v name %q, which that selection may not register", opts, tool)
		}
		if !strings.Contains(got, "[REDACTED]") {
			t.Errorf("instructions for %+v dropped the redaction contract", opts)
		}
		if opts.EnforceScope != strings.Contains(got, "Scope is enforced") {
			t.Errorf("instructions for %+v: scope note presence is wrong", opts)
		}
		if len(got) > maxInstructionsBytes {
			t.Errorf("instructions for %+v are %d bytes, over budget", opts, len(got))
		}
	}
}
