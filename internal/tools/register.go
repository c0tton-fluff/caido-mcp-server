// Package tools registers the MCP tools that let an agent drive Caido:
// proxy history, replay/send, automate, findings, scopes, projects,
// workflows, intercept, tamper rules, filters, and utilities. Each tool
// lives in its own file and is listed in toolGroups.
package tools

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/replay"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerFunc registers a single tool on the server.
type registerFunc func(server *mcp.Server, client *caido.Client)

// toolGroup is a named set of related tools: a toolset. Every tool belongs
// to exactly one, so selecting toolsets (Options.Toolsets) partitions the
// server cleanly.
type toolGroup struct {
	name string
	// about is the one-line description shown in --help and error messages.
	about string
	tools []registerFunc
}

// toolGroups is the canonical list of tool registrations, by toolset.
// Adding a tool here keeps RegisterAll, the startup banner count and
// schema_test in sync automatically -- no separate count to update.
var toolGroups = []toolGroup{
	{
		name:  "history",
		about: "Captured HTTP traffic: list, read, diff, sitemap, body conversion.",
		tools: []registerFunc{
			RegisterListRequestsTool,
			RegisterGetRequestTool,
			RegisterDiffResponsesTool,
			RegisterGetSitemapTool,
			RegisterConvertBodyTool,
		},
	},
	{
		name:  "replay",
		about: "Send requests through Caido Replay and manage its sessions, collections and cookie jars.",
		tools: []registerFunc{
			RegisterSendRequestTool,
			RegisterBatchSendTool,
			RegisterEditRequestTool,
			RegisterExportCurlTool,
			RegisterCreateReplaySessionTool,
			RegisterListReplaySessionsTool,
			RegisterDeleteReplaySessionsTool,
			RegisterMoveReplaySessionTool,
			RegisterRenameReplaySessionTool,
			RegisterGetReplayEntryTool,
			RegisterClearSessionCookiesTool,
			RegisterGetSessionCookiesTool,
			RegisterListReplayCollectionsTool,
			RegisterCreateReplayCollectionTool,
			RegisterRenameReplayCollectionTool,
			RegisterDeleteReplayCollectionTool,
		},
	},
	{
		name:  "websocket",
		about: "WebSocket history and WebSocket replay (connect, send, read, close).",
		tools: []registerFunc{
			RegisterListWsStreamsTool,
			RegisterListWsMessagesTool,
			RegisterWsConnectTool,
			RegisterWsSendTool,
			RegisterWsReadTool,
			RegisterWsCloseTool,
			RegisterSetWsMessageDraftTool,
			RegisterClearWsMessageDraftTool,
		},
	},
	{
		name:  "race",
		about: "Race-condition testing: Replay pipelines and the raw-socket race window (bypasses Caido).",
		tools: []registerFunc{
			RegisterRaceWindowSendTool,
			RegisterRacePipelineTool,
		},
	},
	{
		name:  "scopes",
		about: "Project scopes: list, check a host, create, change, delete.",
		tools: []registerFunc{
			RegisterListScopesTool,
			RegisterIsInScopeTool,
			RegisterCreateScopeTool,
			RegisterRenameScopeTool,
			RegisterUpdateScopeTool,
			RegisterDeleteScopeTool,
		},
	},
	{
		name:  "findings",
		about: "Findings: list, create, delete, export.",
		tools: []registerFunc{
			RegisterListFindingsTool,
			RegisterCreateFindingTool,
			RegisterDeleteFindingsTool,
			RegisterExportFindingsTool,
		},
	},
	{
		name:  "automate",
		about: "Automate (fuzzing) sessions, entries and task control.",
		tools: []registerFunc{
			RegisterListAutomateSessionsTool,
			RegisterGetAutomateSessionTool,
			RegisterGetAutomateEntryTool,
			RegisterAutomateTaskControlTool,
		},
	},
	{
		name:  "intercept",
		about: "Intercept queue and TLS pass-through options.",
		tools: []registerFunc{
			RegisterInterceptStatusTool,
			RegisterInterceptControlTool,
			RegisterListInterceptEntriesTool,
			RegisterForwardInterceptTool,
			RegisterDropInterceptTool,
			RegisterPassthroughTool,
		},
	},
	{
		name:  "tamper",
		about: "Match & Replace rules.",
		tools: []registerFunc{
			RegisterListTamperRulesTool,
			RegisterCreateTamperRuleTool,
			RegisterUpdateTamperRuleTool,
			RegisterTestTamperRuleTool,
			RegisterToggleTamperRuleTool,
			RegisterDeleteTamperRuleTool,
		},
	},
	{
		name:  "workflows",
		about: "Workflows: list, run, toggle.",
		tools: []registerFunc{
			RegisterListWorkflowsTool,
			RegisterRunWorkflowTool,
			RegisterToggleWorkflowTool,
		},
	},
	{
		name:  "environments",
		about: "Environments and their variables.",
		tools: []registerFunc{
			RegisterListEnvironmentsTool,
			RegisterSelectEnvironmentTool,
			RegisterUpdateEnvironmentTool,
			RegisterCreateEnvironmentTool,
			RegisterDeleteEnvironmentTool,
		},
	},
	{
		name:  "admin",
		about: "Instance and project administration: projects, filter presets, hosted files, tasks, plugins.",
		tools: []registerFunc{
			RegisterGetInstanceTool,
			RegisterListProjectsTool,
			RegisterSelectProjectTool,
			RegisterCreateProjectTool,
			RegisterRenameProjectTool,
			RegisterDeleteProjectTool,
			RegisterListFiltersTool,
			RegisterCreateFilterTool,
			RegisterDeleteFilterTool,
			RegisterListHostedFilesTool,
			RegisterListTasksTool,
			RegisterCancelTaskTool,
			RegisterListPluginsTool,
		},
	},
}

// toolsetAliases are shorthand names for a combination of toolsets.
var toolsetAliases = map[string][]string{
	// core is what most testing sessions need: read captured traffic, send
	// requests, respect the scope, record findings.
	"core": {"history", "replay", "scopes", "findings"},
}

// Options selects which tools RegisterAllWith exposes and how they are
// advertised.
type Options struct {
	// ReadOnly exposes only the tools annotated read-only: nothing that
	// sends traffic or changes the Caido project is registered, so a client
	// cannot call it even by name.
	ReadOnly bool
	// EnforceScope refuses sends to hosts outside the project scope and
	// withholds the tools that could change the scope (see scope_enforce.go).
	EnforceScope bool
	// Toolsets limits registration to these toolsets (toolGroups names or
	// toolsetAliases); empty registers every toolset.
	Toolsets []string
	// Compact drops each tool's output schema from the advertised tool
	// list. Results still carry structured content; only the up-front
	// description shrinks (about a third of the list).
	Compact bool
}

// ToolsetNames returns the selectable toolset names and aliases with a
// one-line description each, in declaration order, for help text.
func ToolsetNames() []string {
	out := make([]string, 0, len(toolGroups)+len(toolsetAliases))
	for _, g := range toolGroups {
		out = append(out, g.name+" - "+g.about)
	}
	for _, alias := range slices.Sorted(maps.Keys(toolsetAliases)) {
		out = append(out, alias+" - shorthand for "+strings.Join(toolsetAliases[alias], ","))
	}
	return out
}

// selectGroups resolves toolset names and aliases to groups, in
// toolGroups order and without duplicates. No names selects every group.
// An unknown name is an error: a typo must not silently drop tools the
// operator asked for, or register ones they did not.
func selectGroups(names []string) ([]toolGroup, error) {
	if len(names) == 0 {
		return toolGroups, nil
	}
	want := map[string]bool{}
	for _, raw := range names {
		name := strings.ToLower(strings.TrimSpace(raw))
		if name == "" {
			continue
		}
		if expanded, ok := toolsetAliases[name]; ok {
			for _, n := range expanded {
				want[n] = true
			}
			continue
		}
		if !slices.ContainsFunc(toolGroups, func(g toolGroup) bool { return g.name == name }) {
			valid := make([]string, 0, len(toolGroups))
			for _, g := range toolGroups {
				valid = append(valid, g.name)
			}
			valid = append(valid, slices.Sorted(maps.Keys(toolsetAliases))...)
			return nil, fmt.Errorf("unknown toolset %q (valid: %s)", raw, strings.Join(valid, ", "))
		}
		want[name] = true
	}
	if len(want) == 0 {
		return nil, fmt.Errorf("toolsets is set but names no toolset")
	}
	var groups []toolGroup
	for _, g := range toolGroups {
		if want[g.name] {
			groups = append(groups, g)
		}
	}
	return groups, nil
}

// RegisterAll registers every tool on the server and returns the number
// of tools registered.
func RegisterAll(server *mcp.Server, client *caido.Client) int {
	n, _ := RegisterAllWith(context.Background(), server, client, Options{})
	return n
}

// RegisterAllWith registers the tools opts selects and returns how many
// are exposed. It fails on an unknown toolset, or when the registered tools
// cannot be listed to tell which ones to withhold; the server must not
// start in either case, since it would expose tools the operator excluded.
func RegisterAllWith(
	ctx context.Context, server *mcp.Server, client *caido.Client, opts Options,
) (int, error) {
	groups, err := selectGroups(opts.Toolsets)
	if err != nil {
		return 0, err
	}
	var selected []registerFunc
	for _, g := range groups {
		selected = append(selected, g.tools...)
	}

	// Normalize the input schemas advertised to clients (see
	// normalizeToolSchemas): params must not be ["null", <type>] unions, which
	// several MCP clients mis-serialize.
	server.AddReceivingMiddleware(normalizeToolSchemas(opts.Compact))
	for _, register := range selected {
		register(server, client)
	}

	// The guard is process-wide, so set it either way: the last server
	// configured decides, and "off" really is off.
	replay.SetTargetGuard(nil)
	if opts.EnforceScope {
		replay.SetTargetGuard(enforceScope)
	}
	if !opts.ReadOnly && !opts.EnforceScope {
		return len(selected), nil
	}

	registered, err := probeTools(ctx, client, selected)
	if err != nil {
		return 0, fmt.Errorf("restricting tools: %w", err)
	}
	var withheld []string
	for _, tool := range registered {
		switch {
		case opts.ReadOnly && !tool.readOnly:
			withheld = append(withheld, tool.name)
		case opts.EnforceScope && slices.Contains(scopeLockedTools, tool.name):
			withheld = append(withheld, tool.name)
		}
	}
	server.RemoveTools(withheld...)
	return len(registered) - len(withheld), nil
}

// probedTool is what restricting the tool set needs to know about a tool.
type probedTool struct {
	name     string
	readOnly bool
}

// probeTools returns the name and read-only annotation of every tool the
// given registrations add. Both live in each tool's registration, so they
// are read back from a scratch server over an in-memory session rather than
// kept in a second list that could drift.
func probeTools(
	ctx context.Context, client *caido.Client, registrations []registerFunc,
) ([]probedTool, error) {
	scratch := mcp.NewServer(&mcp.Implementation{Name: "tool-probe", Version: "0"}, nil)
	for _, register := range registrations {
		register(scratch, client)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	serverT, clientT := mcp.NewInMemoryTransports()
	if _, err := scratch.Connect(ctx, serverT, nil); err != nil {
		return nil, fmt.Errorf("probe tools: %w", err)
	}
	session, err := mcp.NewClient(
		&mcp.Implementation{Name: "tool-probe", Version: "0"}, nil,
	).Connect(ctx, clientT, nil)
	if err != nil {
		return nil, fmt.Errorf("probe tools: %w", err)
	}
	defer func() { _ = session.Close() }()

	var tools []probedTool
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			return nil, fmt.Errorf("probe tools: %w", err)
		}
		tools = append(tools, probedTool{
			name:     tool.Name,
			readOnly: tool.Annotations != nil && tool.Annotations.ReadOnlyHint,
		})
	}
	// A short listing would leave tools registered and unnoticed.
	if len(tools) != len(registrations) {
		return nil, fmt.Errorf("probe tools: listed %d of %d tools", len(tools), len(registrations))
	}
	return tools, nil
}
