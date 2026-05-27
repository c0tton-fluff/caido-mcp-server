package tools

import (
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func RegisterAll(server *mcp.Server, client *caido.Client) {
	// HTTP History
	RegisterListRequestsTool(server, client)
	RegisterGetRequestTool(server, client)
	RegisterGetRequestMetadataTool(server, client)

	// WebSocket History
	RegisterListWsStreamsTool(server, client)
	RegisterListWsMessagesTool(server, client)

	// Automate (Fuzzing)
	RegisterListAutomateSessionsTool(server, client)
	RegisterGetAutomateSessionTool(server, client)
	RegisterGetAutomateEntryTool(server, client)
	RegisterAutomateTaskControlTool(server, client)
	RegisterCreateAutomateSessionTool(server, client)
	RegisterRenameAutomateSessionTool(server, client)
	RegisterDeleteAutomateSessionTool(server, client)

	// Replay (Send Requests)
	RegisterSendRequestTool(server, client)
	RegisterBatchSendTool(server, client)
	RegisterEditRequestTool(server, client)
	RegisterExportCurlTool(server, client)
	RegisterCreateReplaySessionTool(server, client)
	RegisterListReplaySessionsTool(server, client)
	RegisterDeleteReplaySessionsTool(server, client)
	RegisterMoveReplaySessionTool(server, client)
	RegisterGetReplayEntryTool(server, client)
	RegisterClearSessionCookiesTool(server, client)
	RegisterGetSessionCookiesTool(server, client)
	RegisterGetReplaySessionTool(server, client)
	RegisterRenameReplaySessionTool(server, client)

	// Replay Collections
	RegisterListReplayCollectionsTool(server, client)
	RegisterCreateReplayCollectionTool(server, client)
	RegisterRenameReplayCollectionTool(server, client)
	RegisterDeleteReplayCollectionTool(server, client)

	// Findings
	RegisterListFindingsTool(server, client)
	RegisterCreateFindingTool(server, client)
	RegisterDeleteFindingsTool(server, client)
	RegisterExportFindingsTool(server, client)
	RegisterListFindingReportersTool(server, client)
	RegisterGetFindingTool(server, client)

	// User
	RegisterWhoamiTool(server, client)

	// Sitemap
	RegisterGetSitemapTool(server, client)
	RegisterGetSitemapEntryTool(server, client)
	RegisterClearSitemapTool(server, client)
	RegisterDeleteSitemapEntriesTool(server, client)

	// Scopes
	RegisterListScopesTool(server, client)
	RegisterCreateScopeTool(server, client)
	RegisterRenameScopeTool(server, client)
	RegisterDeleteScopeTool(server, client)

	// Projects
	RegisterListProjectsTool(server, client)
	RegisterSelectProjectTool(server, client)
	RegisterCreateProjectTool(server, client)
	RegisterRenameProjectTool(server, client)
	RegisterDeleteProjectTool(server, client)

	// Workflows
	RegisterListWorkflowsTool(server, client)
	RegisterRunWorkflowTool(server, client)
	RegisterToggleWorkflowTool(server, client)
	RegisterGetWorkflowTool(server, client)
	RegisterCreateWorkflowTool(server, client)
	RegisterRenameWorkflowTool(server, client)
	RegisterDeleteWorkflowTool(server, client)
	RegisterSetWorkflowScopeTool(server, client)
	RegisterListWorkflowNodeDefinitionsTool(server, client)

	// Environments
	RegisterListEnvironmentsTool(server, client)
	RegisterSelectEnvironmentTool(server, client)
	RegisterCreateEnvironmentTool(server, client)
	RegisterDeleteEnvironmentTool(server, client)

	// Instance
	RegisterGetInstanceTool(server, client)

	// Intercept
	RegisterInterceptStatusTool(server, client)
	RegisterInterceptControlTool(server, client)
	RegisterListInterceptEntriesTool(server, client)
	RegisterForwardInterceptTool(server, client)
	RegisterDropInterceptTool(server, client)
	RegisterGetInterceptEntryTool(server, client)
	RegisterGetInterceptOptionsTool(server, client)
	RegisterSetInterceptOptionsTool(server, client)
	RegisterDeleteInterceptEntryTool(server, client)
	RegisterDeleteInterceptEntriesTool(server, client)

	// Tamper (Match & Replace)
	RegisterListTamperRulesTool(server, client)
	RegisterCreateTamperRuleTool(server, client)
	RegisterUpdateTamperRuleTool(server, client)
	RegisterToggleTamperRuleTool(server, client)
	RegisterDeleteTamperRuleTool(server, client)
	RegisterGetTamperRuleTool(server, client)
	RegisterCreateTamperCollectionTool(server, client)
	RegisterDeleteTamperCollectionTool(server, client)

	// Filters
	RegisterListFiltersTool(server, client)
	RegisterCreateFilterTool(server, client)
	RegisterDeleteFilterTool(server, client)

	// Hosted Files
	RegisterListHostedFilesTool(server, client)
	RegisterRenameHostedFileTool(server, client)
	RegisterDeleteHostedFileTool(server, client)

	// Tasks
	RegisterListTasksTool(server, client)
	RegisterCancelTaskTool(server, client)

	// Plugins
	RegisterListPluginsTool(server, client)
	RegisterInstallPluginTool(server, client)
	RegisterDeletePluginTool(server, client)
	RegisterTogglePluginTool(server, client)
}
