# MCP Tools Expansion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add 8 new MCP tools to caido-mcp-server using existing sdk-go v0.2.2 methods, expanding from traffic viewer to testing assistant.

**Architecture:** Each tool is a standalone file in `internal/tools/` following the existing handler pattern (Input struct, Output struct, handler closure, Register function). Registration is added to `cmd/mcp/serve.go`. No SDK changes needed -- all methods already exist in sdk-go v0.2.2.

**Tech Stack:** Go 1.24, caido-community/sdk-go v0.2.2, modelcontextprotocol/go-sdk v1.2.0

---

### Task 1: List Intercept Entries Tool

**Files:**
- Create: `internal/tools/list_intercept_entries.go`
- Modify: `cmd/mcp/serve.go`

- [ ] **Step 1: Create `internal/tools/list_intercept_entries.go`**

```go
package tools

import (
	"context"
	"fmt"
	"time"

	caido "github.com/caido-community/sdk-go"
	"github.com/c0tton-fluff/caido-mcp-server/internal/httputil"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ListInterceptEntriesInput is the input for the tool
type ListInterceptEntriesInput struct {
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum entries to return (default 20, max 100)"`
	After  string `json:"after,omitempty" jsonschema:"Cursor for pagination"`
	Filter string `json:"filter,omitempty" jsonschema:"HTTPQL filter query"`
}

// InterceptEntrySummary is a minimal intercept entry
type InterceptEntrySummary struct {
	ID         string `json:"id"`
	RequestID  string `json:"requestId"`
	Method     string `json:"method"`
	URL        string `json:"url"`
	StatusCode int    `json:"statusCode,omitempty"`
	CreatedAt  string `json:"createdAt"`
}

// ListInterceptEntriesOutput is the output
type ListInterceptEntriesOutput struct {
	Entries    []InterceptEntrySummary `json:"entries"`
	HasMore    bool                    `json:"hasMore"`
	NextCursor string                  `json:"nextCursor,omitempty"`
	Total      int                     `json:"total"`
}

func listInterceptEntriesHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, ListInterceptEntriesInput) (*mcp.CallToolResult, ListInterceptEntriesOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input ListInterceptEntriesInput,
	) (*mcp.CallToolResult, ListInterceptEntriesOutput, error) {
		limit := input.Limit
		if limit <= 0 {
			limit = 20
		}
		if limit > 100 {
			limit = 100
		}

		opts := &caido.ListInterceptEntriesOptions{
			First: &limit,
		}
		if input.Filter != "" {
			opts.Filter = &input.Filter
		}
		if input.After != "" {
			opts.After = &input.After
		}

		resp, err := client.Intercept.ListEntries(ctx, opts)
		if err != nil {
			return nil, ListInterceptEntriesOutput{}, fmt.Errorf(
				"failed to list intercept entries: %w", err,
			)
		}

		conn := resp.InterceptEntries
		output := ListInterceptEntriesOutput{
			Entries: make(
				[]InterceptEntrySummary, 0, len(conn.Edges),
			),
		}

		if conn.Count != nil {
			output.Total = conn.Count.Value
		}

		for _, edge := range conn.Edges {
			e := edge.Node
			if e.Request == nil {
				continue
			}
			r := e.Request
			summary := InterceptEntrySummary{
				ID:        e.Id,
				RequestID: r.Id,
				Method:    r.Method,
				URL: httputil.BuildURL(
					r.IsTls, r.Host, r.Port, r.Path, r.Query,
				),
				CreatedAt: time.UnixMilli(r.CreatedAt).Format(
					time.RFC3339,
				),
			}
			if r.Response != nil {
				summary.StatusCode = r.Response.StatusCode
			}
			output.Entries = append(output.Entries, summary)
		}

		if conn.PageInfo != nil && conn.PageInfo.HasNextPage {
			output.HasMore = true
			if conn.PageInfo.EndCursor != nil {
				output.NextCursor = *conn.PageInfo.EndCursor
			}
		}

		return nil, output, nil
	}
}

// RegisterListInterceptEntriesTool registers the tool
func RegisterListInterceptEntriesTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_list_intercept_entries",
		Description: `List queued intercept entries. Filter with httpql. Returns id/method/url/status. Use with forward/drop tools.`,
	}, listInterceptEntriesHandler(client))
}
```

- [ ] **Step 2: Register in `cmd/mcp/serve.go`**

Add after the existing `tools.RegisterInterceptControlTool(server, client)` line:

```go
	tools.RegisterListInterceptEntriesTool(server, client)
```

- [ ] **Step 3: Build and verify**

Run: `cd /Users/mambrozkiewicz/Documents/Caido-Repo && go build ./cmd/mcp/`
Expected: Clean build, no errors

- [ ] **Step 4: Commit**

```bash
git add internal/tools/list_intercept_entries.go cmd/mcp/serve.go
git commit -m "feat: add caido_list_intercept_entries tool"
```

---

### Task 2: Forward Intercept Tool

**Files:**
- Create: `internal/tools/forward_intercept.go`
- Modify: `cmd/mcp/serve.go`

- [ ] **Step 1: Create `internal/tools/forward_intercept.go`**

```go
package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	gen "github.com/caido-community/sdk-go/graphql"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ForwardInterceptInput is the input for the tool
type ForwardInterceptInput struct {
	ID  string `json:"id" jsonschema:"required,Intercept entry ID to forward"`
	Raw string `json:"raw,omitempty" jsonschema:"Modified raw HTTP request (base64-encoded). Omit to forward unmodified."`
}

// ForwardInterceptOutput is the output
type ForwardInterceptOutput struct {
	ForwardedID string `json:"forwardedId"`
}

func forwardInterceptHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, ForwardInterceptInput) (*mcp.CallToolResult, ForwardInterceptOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input ForwardInterceptInput,
	) (*mcp.CallToolResult, ForwardInterceptOutput, error) {
		if input.ID == "" {
			return nil, ForwardInterceptOutput{}, fmt.Errorf(
				"id is required",
			)
		}

		var fwdInput *gen.ForwardInterceptMessageInput
		if input.Raw != "" {
			fwdInput = &gen.ForwardInterceptMessageInput{
				Request: &gen.ForwardInterceptRequestMessageInput{
					UpdateRaw:           input.Raw,
					UpdateContentLength: true,
				},
			}
		}

		resp, err := client.Intercept.Forward(
			ctx, input.ID, fwdInput,
		)
		if err != nil {
			return nil, ForwardInterceptOutput{}, err
		}

		return nil, ForwardInterceptOutput{
			ForwardedID: resp.ForwardInterceptMessage.ForwardedId,
		}, nil
	}
}

// RegisterForwardInterceptTool registers the tool
func RegisterForwardInterceptTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_forward_intercept",
		Description: `Forward intercepted request. Optionally modify with base64-encoded raw HTTP request. Params: id (required), raw (optional).`,
	}, forwardInterceptHandler(client))
}
```

- [ ] **Step 2: Register in `cmd/mcp/serve.go`**

Add after `tools.RegisterListInterceptEntriesTool(server, client)`:

```go
	tools.RegisterForwardInterceptTool(server, client)
```

- [ ] **Step 3: Build and verify**

Run: `cd /Users/mambrozkiewicz/Documents/Caido-Repo && go build ./cmd/mcp/`
Expected: Clean build, no errors

- [ ] **Step 4: Commit**

```bash
git add internal/tools/forward_intercept.go cmd/mcp/serve.go
git commit -m "feat: add caido_forward_intercept tool"
```

---

### Task 3: Drop Intercept Tool

**Files:**
- Create: `internal/tools/drop_intercept.go`
- Modify: `cmd/mcp/serve.go`

- [ ] **Step 1: Create `internal/tools/drop_intercept.go`**

```go
package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// DropInterceptInput is the input for the tool
type DropInterceptInput struct {
	ID string `json:"id" jsonschema:"required,Intercept entry ID to drop"`
}

// DropInterceptOutput is the output
type DropInterceptOutput struct {
	DroppedID string `json:"droppedId"`
}

func dropInterceptHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, DropInterceptInput) (*mcp.CallToolResult, DropInterceptOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input DropInterceptInput,
	) (*mcp.CallToolResult, DropInterceptOutput, error) {
		if input.ID == "" {
			return nil, DropInterceptOutput{}, fmt.Errorf(
				"id is required",
			)
		}

		resp, err := client.Intercept.Drop(ctx, input.ID)
		if err != nil {
			return nil, DropInterceptOutput{}, err
		}

		return nil, DropInterceptOutput{
			DroppedID: resp.DropInterceptMessage.DroppedId,
		}, nil
	}
}

// RegisterDropInterceptTool registers the tool
func RegisterDropInterceptTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_drop_intercept",
		Description: `Drop intercepted request (do not forward). Params: id (required).`,
	}, dropInterceptHandler(client))
}
```

- [ ] **Step 2: Register in `cmd/mcp/serve.go`**

Add after `tools.RegisterForwardInterceptTool(server, client)`:

```go
	tools.RegisterDropInterceptTool(server, client)
```

- [ ] **Step 3: Build and verify**

Run: `cd /Users/mambrozkiewicz/Documents/Caido-Repo && go build ./cmd/mcp/`
Expected: Clean build, no errors

- [ ] **Step 4: Commit**

```bash
git add internal/tools/drop_intercept.go cmd/mcp/serve.go
git commit -m "feat: add caido_drop_intercept tool"
```

---

### Task 4: Automate Task Control Tool

**Files:**
- Create: `internal/tools/automate_task_control.go`
- Modify: `cmd/mcp/serve.go`

- [ ] **Step 1: Create `internal/tools/automate_task_control.go`**

```go
package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// AutomateTaskControlInput is the input for the tool
type AutomateTaskControlInput struct {
	Action    string `json:"action" jsonschema:"required,Action: start, pause, resume, or cancel"`
	SessionID string `json:"session_id,omitempty" jsonschema:"Automate session ID (required for start)"`
	TaskID    string `json:"task_id,omitempty" jsonschema:"Automate task ID (required for pause/resume/cancel)"`
}

// AutomateTaskControlOutput is the output
type AutomateTaskControlOutput struct {
	Action string `json:"action"`
	TaskID string `json:"taskId"`
}

func automateTaskControlHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, AutomateTaskControlInput) (*mcp.CallToolResult, AutomateTaskControlOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input AutomateTaskControlInput,
	) (*mcp.CallToolResult, AutomateTaskControlOutput, error) {
		switch input.Action {
		case "start":
			if input.SessionID == "" {
				return nil, AutomateTaskControlOutput{}, fmt.Errorf(
					"session_id is required for start action",
				)
			}
			resp, err := client.Automate.StartTask(
				ctx, input.SessionID,
			)
			if err != nil {
				return nil, AutomateTaskControlOutput{}, err
			}
			return nil, AutomateTaskControlOutput{
				Action: "start",
				TaskID: resp.StartAutomateTask.AutomateTask.Id,
			}, nil

		case "pause":
			if input.TaskID == "" {
				return nil, AutomateTaskControlOutput{}, fmt.Errorf(
					"task_id is required for pause action",
				)
			}
			resp, err := client.Automate.PauseTask(
				ctx, input.TaskID,
			)
			if err != nil {
				return nil, AutomateTaskControlOutput{}, err
			}
			return nil, AutomateTaskControlOutput{
				Action: "pause",
				TaskID: resp.PauseAutomateTask.AutomateTask.Id,
			}, nil

		case "resume":
			if input.TaskID == "" {
				return nil, AutomateTaskControlOutput{}, fmt.Errorf(
					"task_id is required for resume action",
				)
			}
			resp, err := client.Automate.ResumeTask(
				ctx, input.TaskID,
			)
			if err != nil {
				return nil, AutomateTaskControlOutput{}, err
			}
			return nil, AutomateTaskControlOutput{
				Action: "resume",
				TaskID: resp.ResumeAutomateTask.AutomateTask.Id,
			}, nil

		case "cancel":
			if input.TaskID == "" {
				return nil, AutomateTaskControlOutput{}, fmt.Errorf(
					"task_id is required for cancel action",
				)
			}
			resp, err := client.Automate.CancelTask(
				ctx, input.TaskID,
			)
			if err != nil {
				return nil, AutomateTaskControlOutput{}, err
			}
			return nil, AutomateTaskControlOutput{
				Action: "cancel",
				TaskID: resp.CancelAutomateTask.CancelledId,
			}, nil

		default:
			return nil, AutomateTaskControlOutput{}, fmt.Errorf(
				"action must be start, pause, resume, or cancel",
			)
		}
	}
}

// RegisterAutomateTaskControlTool registers the tool
func RegisterAutomateTaskControlTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_automate_task_control",
		Description: `Control fuzzing tasks. Actions: start (needs session_id), pause/resume/cancel (needs task_id).`,
	}, automateTaskControlHandler(client))
}
```

- [ ] **Step 2: Register in `cmd/mcp/serve.go`**

Add after `tools.RegisterGetAutomateEntryTool(server, client)`:

```go
	tools.RegisterAutomateTaskControlTool(server, client)
```

- [ ] **Step 3: Build and verify**

Run: `cd /Users/mambrozkiewicz/Documents/Caido-Repo && go build ./cmd/mcp/`
Expected: Clean build, no errors

- [ ] **Step 4: Commit**

```bash
git add internal/tools/automate_task_control.go cmd/mcp/serve.go
git commit -m "feat: add caido_automate_task_control tool"
```

---

### Task 5: List Environments Tool

**Files:**
- Create: `internal/tools/list_environments.go`
- Modify: `cmd/mcp/serve.go`

- [ ] **Step 1: Create `internal/tools/list_environments.go`**

```go
package tools

import (
	"context"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ListEnvironmentsInput is the input for the tool
type ListEnvironmentsInput struct{}

// EnvironmentVariable is a variable in an environment
type EnvironmentVariable struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Kind  string `json:"kind"`
}

// EnvironmentSummary is a summary of an environment
type EnvironmentSummary struct {
	ID        string                `json:"id"`
	Name      string                `json:"name"`
	Variables []EnvironmentVariable `json:"variables"`
}

// ListEnvironmentsOutput is the output
type ListEnvironmentsOutput struct {
	Environments []EnvironmentSummary `json:"environments"`
	GlobalID     string               `json:"globalId,omitempty"`
	SelectedID   string               `json:"selectedId,omitempty"`
}

func listEnvironmentsHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, ListEnvironmentsInput) (*mcp.CallToolResult, ListEnvironmentsOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input ListEnvironmentsInput,
	) (*mcp.CallToolResult, ListEnvironmentsOutput, error) {
		listResp, err := client.Environment.List(ctx)
		if err != nil {
			return nil, ListEnvironmentsOutput{}, err
		}

		ctxResp, err := client.Environment.GetContext(ctx)
		if err != nil {
			return nil, ListEnvironmentsOutput{}, err
		}

		output := ListEnvironmentsOutput{
			Environments: make(
				[]EnvironmentSummary, 0,
				len(listResp.Environments),
			),
		}

		if ctxResp.EnvironmentContext.Global != nil {
			output.GlobalID = ctxResp.EnvironmentContext.Global.Id
		}
		if ctxResp.EnvironmentContext.Selected != nil {
			output.SelectedID = ctxResp.EnvironmentContext.Selected.Id
		}

		for _, env := range listResp.Environments {
			summary := EnvironmentSummary{
				ID:   env.Id,
				Name: env.Name,
				Variables: make(
					[]EnvironmentVariable, 0,
					len(env.Variables),
				),
			}
			for _, v := range env.Variables {
				summary.Variables = append(
					summary.Variables,
					EnvironmentVariable{
						Name:  v.Name,
						Value: v.Value,
						Kind:  string(v.Kind),
					},
				)
			}
			output.Environments = append(
				output.Environments, summary,
			)
		}

		return nil, output, nil
	}
}

// RegisterListEnvironmentsTool registers the tool
func RegisterListEnvironmentsTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_list_environments",
		Description: `List environments and their variables (tokens, keys, etc). Shows which is currently selected.`,
	}, listEnvironmentsHandler(client))
}
```

- [ ] **Step 2: Register in `cmd/mcp/serve.go`**

Add a new section after the Workflows section:

```go
	// Environments
	tools.RegisterListEnvironmentsTool(server, client)
```

- [ ] **Step 3: Build and verify**

Run: `cd /Users/mambrozkiewicz/Documents/Caido-Repo && go build ./cmd/mcp/`
Expected: Clean build, no errors

- [ ] **Step 4: Commit**

```bash
git add internal/tools/list_environments.go cmd/mcp/serve.go
git commit -m "feat: add caido_list_environments tool"
```

---

### Task 6: Select Environment Tool

**Files:**
- Create: `internal/tools/select_environment.go`
- Modify: `cmd/mcp/serve.go`

- [ ] **Step 1: Create `internal/tools/select_environment.go`**

```go
package tools

import (
	"context"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// SelectEnvironmentInput is the input for the tool
type SelectEnvironmentInput struct {
	ID string `json:"id" jsonschema:"required,Environment ID to select. Pass empty string to deselect."`
}

// SelectEnvironmentOutput is the output
type SelectEnvironmentOutput struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

func selectEnvironmentHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, SelectEnvironmentInput) (*mcp.CallToolResult, SelectEnvironmentOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input SelectEnvironmentInput,
	) (*mcp.CallToolResult, SelectEnvironmentOutput, error) {
		var idPtr *string
		if input.ID != "" {
			idPtr = &input.ID
		}

		resp, err := client.Environment.Select(ctx, idPtr)
		if err != nil {
			return nil, SelectEnvironmentOutput{}, err
		}

		env := resp.SelectEnvironment.Environment
		if env == nil {
			return nil, SelectEnvironmentOutput{}, nil
		}

		return nil, SelectEnvironmentOutput{
			ID:   env.Id,
			Name: env.Name,
		}, nil
	}
}

// RegisterSelectEnvironmentTool registers the tool
func RegisterSelectEnvironmentTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_select_environment",
		Description: `Select active environment. Variables from selected env are used in replay placeholders. Pass empty id to deselect.`,
	}, selectEnvironmentHandler(client))
}
```

- [ ] **Step 2: Register in `cmd/mcp/serve.go`**

Add after `tools.RegisterListEnvironmentsTool(server, client)`:

```go
	tools.RegisterSelectEnvironmentTool(server, client)
```

- [ ] **Step 3: Build and verify**

Run: `cd /Users/mambrozkiewicz/Documents/Caido-Repo && go build ./cmd/mcp/`
Expected: Clean build, no errors

- [ ] **Step 4: Commit**

```bash
git add internal/tools/select_environment.go cmd/mcp/serve.go
git commit -m "feat: add caido_select_environment tool"
```

---

### Task 7: Delete Findings Tool

**Files:**
- Create: `internal/tools/delete_findings.go`
- Modify: `cmd/mcp/serve.go`

- [ ] **Step 1: Create `internal/tools/delete_findings.go`**

```go
package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	gen "github.com/caido-community/sdk-go/graphql"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// DeleteFindingsInput is the input for the tool
type DeleteFindingsInput struct {
	IDs      []string `json:"ids,omitempty" jsonschema:"List of finding IDs to delete"`
	Reporter string   `json:"reporter,omitempty" jsonschema:"Delete all findings by this reporter name"`
}

// DeleteFindingsOutput is the output
type DeleteFindingsOutput struct {
	DeletedIDs []string `json:"deletedIds"`
}

func deleteFindingsHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, DeleteFindingsInput) (*mcp.CallToolResult, DeleteFindingsOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input DeleteFindingsInput,
	) (*mcp.CallToolResult, DeleteFindingsOutput, error) {
		if len(input.IDs) == 0 && input.Reporter == "" {
			return nil, DeleteFindingsOutput{}, fmt.Errorf(
				"provide either ids or reporter",
			)
		}

		delInput := &gen.DeleteFindingsInput{}
		if len(input.IDs) > 0 {
			delInput.Ids = input.IDs
		} else {
			delInput.Reporter = &input.Reporter
		}

		resp, err := client.Findings.Delete(ctx, delInput)
		if err != nil {
			return nil, DeleteFindingsOutput{}, err
		}

		return nil, DeleteFindingsOutput{
			DeletedIDs: resp.DeleteFindings.DeletedIds,
		}, nil
	}
}

// RegisterDeleteFindingsTool registers the tool
func RegisterDeleteFindingsTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_delete_findings",
		Description: `Delete findings by IDs or by reporter name. Params: ids (list) or reporter (string).`,
	}, deleteFindingsHandler(client))
}
```

- [ ] **Step 2: Register in `cmd/mcp/serve.go`**

Add after `tools.RegisterCreateFindingTool(server, client)`:

```go
	tools.RegisterDeleteFindingsTool(server, client)
```

- [ ] **Step 3: Build and verify**

Run: `cd /Users/mambrozkiewicz/Documents/Caido-Repo && go build ./cmd/mcp/`
Expected: Clean build, no errors

- [ ] **Step 4: Commit**

```bash
git add internal/tools/delete_findings.go cmd/mcp/serve.go
git commit -m "feat: add caido_delete_findings tool"
```

---

### Task 8: Export Findings Tool

**Files:**
- Create: `internal/tools/export_findings.go`
- Modify: `cmd/mcp/serve.go`

- [ ] **Step 1: Create `internal/tools/export_findings.go`**

```go
package tools

import (
	"context"
	"fmt"

	caido "github.com/caido-community/sdk-go"
	gen "github.com/caido-community/sdk-go/graphql"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ExportFindingsInput is the input for the tool
type ExportFindingsInput struct {
	IDs      []string `json:"ids,omitempty" jsonschema:"List of finding IDs to export"`
	Reporter string   `json:"reporter,omitempty" jsonschema:"Export all findings by this reporter name"`
}

// ExportFindingsOutput is the output
type ExportFindingsOutput struct {
	ExportID string `json:"exportId"`
}

func exportFindingsHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, ExportFindingsInput) (*mcp.CallToolResult, ExportFindingsOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input ExportFindingsInput,
	) (*mcp.CallToolResult, ExportFindingsOutput, error) {
		if len(input.IDs) == 0 && input.Reporter == "" {
			return nil, ExportFindingsOutput{}, fmt.Errorf(
				"provide either ids or reporter",
			)
		}

		expInput := &gen.ExportFindingsInput{}
		if len(input.IDs) > 0 {
			expInput.Ids = input.IDs
		} else {
			expInput.Filter = &gen.FilterClauseFindingInput{
				Reporter: &input.Reporter,
			}
		}

		resp, err := client.Findings.Export(ctx, expInput)
		if err != nil {
			return nil, ExportFindingsOutput{}, err
		}

		payload := resp.ExportFindings
		if payload.Error != nil {
			return nil, ExportFindingsOutput{}, fmt.Errorf(
				"export findings failed",
			)
		}

		return nil, ExportFindingsOutput{
			ExportID: payload.Export.Id,
		}, nil
	}
}

// RegisterExportFindingsTool registers the tool
func RegisterExportFindingsTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_export_findings",
		Description: `Export findings. Filter by IDs or reporter name. Returns exportId for download.`,
	}, exportFindingsHandler(client))
}
```

- [ ] **Step 2: Register in `cmd/mcp/serve.go`**

Add after `tools.RegisterDeleteFindingsTool(server, client)`:

```go
	tools.RegisterExportFindingsTool(server, client)
```

- [ ] **Step 3: Build and verify**

Run: `cd /Users/mambrozkiewicz/Documents/Caido-Repo && go build ./cmd/mcp/`
Expected: Clean build, no errors

- [ ] **Step 4: Commit**

```bash
git add internal/tools/export_findings.go cmd/mcp/serve.go
git commit -m "feat: add caido_export_findings tool"
```

---

### Task 9: Update README

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Add new tools to the README tool reference table**

Add the following entries to the appropriate sections of the tools table in `README.md`:

Under Intercept section:
- `caido_list_intercept_entries` - List queued intercept entries with HTTPQL filtering
- `caido_forward_intercept` - Forward intercepted request, optionally with modifications
- `caido_drop_intercept` - Drop intercepted request

Under Automate section:
- `caido_automate_task_control` - Start/pause/resume/cancel fuzzing tasks

New Environments section:
- `caido_list_environments` - List environments and their variables
- `caido_select_environment` - Switch active environment

Under Findings section:
- `caido_delete_findings` - Delete findings by IDs or reporter
- `caido_export_findings` - Export findings for reporting

- [ ] **Step 2: Build full project to confirm nothing is broken**

Run: `cd /Users/mambrozkiewicz/Documents/Caido-Repo && go build ./... && go vet ./...`
Expected: Clean build and vet, no warnings

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: add Phase 1 tools to README"
```

---

### Task 10: Final Verification

- [ ] **Step 1: Run full build**

Run: `cd /Users/mambrozkiewicz/Documents/Caido-Repo && go build ./... && go vet ./...`
Expected: Clean build, no errors or warnings

- [ ] **Step 2: Run existing tests**

Run: `cd /Users/mambrozkiewicz/Documents/Caido-Repo && go test ./...`
Expected: All existing tests pass

- [ ] **Step 3: Verify tool count**

Run: `cd /Users/mambrozkiewicz/Documents/Caido-Repo && grep -c 'tools.Register' cmd/mcp/serve.go`
Expected: `28` (20 existing + 8 new)

- [ ] **Step 4: Verify all new files exist**

Run: `ls -1 internal/tools/{list_intercept_entries,forward_intercept,drop_intercept,automate_task_control,list_environments,select_environment,delete_findings,export_findings}.go`
Expected: All 8 files listed
