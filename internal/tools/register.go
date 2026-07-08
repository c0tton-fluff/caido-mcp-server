// Package tools registers the MCP tools that let an agent drive Caido:
// proxy history, replay/send, automate, findings, scopes, projects,
// workflows, intercept, tamper rules, filters, and utilities. Each tool
// lives in its own file and is listed in allTools.
package tools

import (
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

	// Utilities
	RegisterConvertBodyTool,

	// Race-condition testing (bypasses Caido proxy)
	RegisterRaceWindowSendTool,
}

// RegisterAll registers every tool on the server and returns the number
// of tools registered.
func RegisterAll(server *mcp.Server, client *caido.Client) int {
	for _, register := range allTools {
		register(server, client)
	}
	return len(allTools)
}
