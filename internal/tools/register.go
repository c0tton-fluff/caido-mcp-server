// Package tools registers the MCP tools that let an agent drive Caido:
// proxy history, replay/send, automate, findings, scopes, projects,
// workflows, intercept, tamper rules, filters, and utilities. Each tool
// lives in its own file and is listed in allTools.
package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerFunc registers a single tool on the server.
type registerFunc func(server *mcp.Server, client *caido.Client)

// allTools is the canonical list of tool registrations. Adding a tool here
// keeps RegisterAll, the startup banner count, and schema_test in sync
// automatically -- no separate count to update.
var allTools = []registerFunc{
	// HTTP History
	RegisterListRequestsTool,
	RegisterGetRequestTool,
	RegisterDiffResponsesTool,

	// Automate (Fuzzing)
	RegisterListAutomateSessionsTool,
	RegisterGetAutomateSessionTool,
	RegisterGetAutomateEntryTool,
	RegisterAutomateTaskControlTool,

	// Replay (Send Requests)
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

	// Replay Collections
	RegisterListReplayCollectionsTool,
	RegisterCreateReplayCollectionTool,
	RegisterRenameReplayCollectionTool,
	RegisterDeleteReplayCollectionTool,

	// Findings
	RegisterListFindingsTool,
	RegisterCreateFindingTool,
	RegisterDeleteFindingsTool,
	RegisterExportFindingsTool,

	// Sitemap
	RegisterGetSitemapTool,

	// Scopes
	RegisterListScopesTool,
	RegisterIsInScopeTool,
	RegisterCreateScopeTool,
	RegisterRenameScopeTool,
	RegisterUpdateScopeTool,
	RegisterDeleteScopeTool,

	// Projects
	RegisterListProjectsTool,
	RegisterSelectProjectTool,
	RegisterCreateProjectTool,
	RegisterRenameProjectTool,
	RegisterDeleteProjectTool,

	// Workflows
	RegisterListWorkflowsTool,
	RegisterRunWorkflowTool,
	RegisterToggleWorkflowTool,

	// Environments
	RegisterListEnvironmentsTool,
	RegisterSelectEnvironmentTool,
	RegisterUpdateEnvironmentTool,
	RegisterCreateEnvironmentTool,
	RegisterDeleteEnvironmentTool,

	// Instance
	RegisterGetInstanceTool,

	// Intercept
	RegisterInterceptStatusTool,
	RegisterInterceptControlTool,
	RegisterListInterceptEntriesTool,
	RegisterForwardInterceptTool,
	RegisterDropInterceptTool,

	// Tamper (Match & Replace)
	RegisterListTamperRulesTool,
	RegisterCreateTamperRuleTool,
	RegisterUpdateTamperRuleTool,
	RegisterTestTamperRuleTool,
	RegisterToggleTamperRuleTool,
	RegisterDeleteTamperRuleTool,

	// Filters
	RegisterListFiltersTool,
	RegisterCreateFilterTool,
	RegisterDeleteFilterTool,

	// Hosted Files
	RegisterListHostedFilesTool,

	// Tasks
	RegisterListTasksTool,
	RegisterCancelTaskTool,

	// Plugins
	RegisterListPluginsTool,

	// WebSocket History (read)
	RegisterListWsStreamsTool,
	RegisterListWsMessagesTool,

	// WebSocket Replay (connect, send, read, close)
	RegisterWsConnectTool,
	RegisterWsSendTool,
	RegisterWsReadTool,
	RegisterWsCloseTool,

	// Utilities
	RegisterConvertBodyTool,

	// Race-condition testing (bypasses Caido proxy)
	RegisterRaceWindowSendTool,
	RegisterRacePipelineTool,
	RegisterPassthroughTool,
}

// Options selects which tools RegisterAllWith exposes.
type Options struct {
	// ReadOnly exposes only the tools annotated read-only: nothing that
	// sends traffic or changes the Caido project is registered, so a client
	// cannot call it even by name.
	ReadOnly bool
}

// RegisterAll registers every tool on the server and returns the number
// of tools registered.
func RegisterAll(server *mcp.Server, client *caido.Client) int {
	n, _ := RegisterAllWith(context.Background(), server, client, Options{})
	return n
}

// RegisterAllWith registers the tools opts selects and returns how many
// are exposed. It only fails in read-only mode, when the registered tools
// cannot be listed to tell which ones to withhold; the server must not
// start in that case, since it would expose the mutating tools.
func RegisterAllWith(
	ctx context.Context, server *mcp.Server, client *caido.Client, opts Options,
) (int, error) {
	// Normalize the input schemas advertised to clients (see
	// normalizeToolSchemas): params must not be ["null", <type>] unions, which
	// several MCP clients mis-serialize.
	server.AddReceivingMiddleware(normalizeToolSchemas())
	for _, register := range allTools {
		register(server, client)
	}
	if !opts.ReadOnly {
		return len(allTools), nil
	}
	mutating, err := mutatingToolNames(ctx, client)
	if err != nil {
		return 0, fmt.Errorf("read-only mode: %w", err)
	}
	server.RemoveTools(mutating...)
	return len(allTools) - len(mutating), nil
}

// mutatingToolNames returns the names of the tools not annotated
// read-only. The annotation lives in each tool's registration, so the names
// are read back from a scratch server over an in-memory session rather than
// kept in a second list that could drift.
func mutatingToolNames(ctx context.Context, client *caido.Client) ([]string, error) {
	scratch := mcp.NewServer(&mcp.Implementation{Name: "tool-probe", Version: "0"}, nil)
	for _, register := range allTools {
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

	var names []string
	seen := 0
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			return nil, fmt.Errorf("probe tools: %w", err)
		}
		seen++
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			names = append(names, tool.Name)
		}
	}
	// A short listing would leave mutating tools registered and unnoticed.
	if seen != len(allTools) {
		return nil, fmt.Errorf("probe tools: listed %d of %d tools", seen, len(allTools))
	}
	return names, nil
}
