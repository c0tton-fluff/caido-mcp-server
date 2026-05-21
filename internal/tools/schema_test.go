package tools_test

import (
	"context"
	"testing"

	"github.com/c0tton-fluff/caido-mcp-server/internal/testutil"
	"github.com/c0tton-fluff/caido-mcp-server/internal/tools"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var allRegistrations = []struct {
	name     string
	register func(*mcp.Server, *caido.Client)
}{
	{"caido_list_requests", tools.RegisterListRequestsTool},
	{"caido_get_request", tools.RegisterGetRequestTool},
	{"caido_send_request", tools.RegisterSendRequestTool},
	{"caido_batch_send", tools.RegisterBatchSendTool},
	{"caido_create_replay_session", tools.RegisterCreateReplaySessionTool},
	{"caido_list_replay_sessions", tools.RegisterListReplaySessionsTool},
	{"caido_get_replay_entry", tools.RegisterGetReplayEntryTool},
	{"caido_list_automate_sessions", tools.RegisterListAutomateSessionsTool},
	{"caido_get_automate_session", tools.RegisterGetAutomateSessionTool},
	{"caido_get_automate_entry", tools.RegisterGetAutomateEntryTool},
	{"caido_automate_task_control", tools.RegisterAutomateTaskControlTool},
	{"caido_list_findings", tools.RegisterListFindingsTool},
	{"caido_create_finding", tools.RegisterCreateFindingTool},
	{"caido_delete_findings", tools.RegisterDeleteFindingsTool},
	{"caido_export_findings", tools.RegisterExportFindingsTool},
	{"caido_get_sitemap", tools.RegisterGetSitemapTool},
	{"caido_list_scopes", tools.RegisterListScopesTool},
	{"caido_create_scope", tools.RegisterCreateScopeTool},
	{"caido_list_projects", tools.RegisterListProjectsTool},
	{"caido_select_project", tools.RegisterSelectProjectTool},
	{"caido_list_workflows", tools.RegisterListWorkflowsTool},
	{"caido_run_workflow", tools.RegisterRunWorkflowTool},
	{"caido_toggle_workflow", tools.RegisterToggleWorkflowTool},
	{"caido_list_environments", tools.RegisterListEnvironmentsTool},
	{"caido_select_environment", tools.RegisterSelectEnvironmentTool},
	{"caido_get_instance", tools.RegisterGetInstanceTool},
	{"caido_intercept_status", tools.RegisterInterceptStatusTool},
	{"caido_intercept_control", tools.RegisterInterceptControlTool},
	{"caido_list_intercept_entries", tools.RegisterListInterceptEntriesTool},
	{"caido_forward_intercept", tools.RegisterForwardInterceptTool},
	{"caido_drop_intercept", tools.RegisterDropInterceptTool},
	{"caido_list_tamper_rules", tools.RegisterListTamperRulesTool},
	{"caido_create_tamper_rule", tools.RegisterCreateTamperRuleTool},
	{"caido_update_tamper_rule", tools.RegisterUpdateTamperRuleTool},
	{"caido_toggle_tamper_rule", tools.RegisterToggleTamperRuleTool},
	{"caido_delete_tamper_rule", tools.RegisterDeleteTamperRuleTool},
	{"caido_list_filters", tools.RegisterListFiltersTool},
	{"caido_create_filter", tools.RegisterCreateFilterTool},
	{"caido_delete_filter", tools.RegisterDeleteFilterTool},
	{"caido_list_hosted_files", tools.RegisterListHostedFilesTool},
	{"caido_list_tasks", tools.RegisterListTasksTool},
	{"caido_cancel_task", tools.RegisterCancelTaskTool},
	{"caido_list_plugins", tools.RegisterListPluginsTool},
}

func TestAllToolsRegisterAndListable(t *testing.T) {
	env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
		for _, reg := range allRegistrations {
			reg.register(s, c)
		}
	})

	result, err := env.MCPClient.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}

	registered := make(map[string]bool)
	for _, tool := range result.Tools {
		registered[tool.Name] = true
		if tool.Description == "" {
			t.Errorf("tool %q has empty description", tool.Name)
		}
		if tool.InputSchema == nil {
			t.Errorf("tool %q has nil InputSchema", tool.Name)
		}
	}

	for _, reg := range allRegistrations {
		if !registered[reg.name] {
			t.Errorf("tool %q not found in ListTools response", reg.name)
		}
	}
}

func TestToolCount(t *testing.T) {
	env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
		for _, reg := range allRegistrations {
			reg.register(s, c)
		}
	})

	result, err := env.MCPClient.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}

	expected := len(allRegistrations)
	if len(result.Tools) != expected {
		t.Fatalf("want %d tools registered, got %d", expected, len(result.Tools))
	}
}
