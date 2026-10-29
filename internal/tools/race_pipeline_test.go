package tools_test

import (
	"encoding/base64"
	"strconv"
	"testing"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/testutil"
	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/tools"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func pipelineCreated(id string) map[string]any {
	return map[string]any{"createReplayPipelineHttpOneSession": map[string]any{
		"session": map[string]any{"id": id}, "error": nil,
	}}
}

func pipelineStarted(entryID string) map[string]any {
	return map[string]any{"startReplayTask": map[string]any{
		"task":  map[string]any{"id": "task-1", "replayEntry": map[string]any{"id": entryID}},
		"error": nil,
	}}
}

// pipelineHTTPEntry is one resolved request; status 0 means no response yet.
func pipelineHTTPEntry(id string, status int, body string) map[string]any {
	e := map[string]any{"id": id, "error": nil, "request": nil}
	if status != 0 {
		raw := "HTTP/1.1 " + strconv.Itoa(status) + " X\r\nContent-Length: " +
			strconv.Itoa(len(body)) + "\r\n\r\n" + body
		e["request"] = map[string]any{
			"id": "req-" + id,
			"response": map[string]any{
				"statusCode": status, "roundtripTime": 12, "length": len(raw),
				"raw": base64.StdEncoding.EncodeToString([]byte(raw)),
			},
		}
	}
	return e
}

func pipelineEntry(id string, httpEntries ...map[string]any) map[string]any {
	if httpEntries == nil {
		httpEntries = []map[string]any{}
	}
	return map[string]any{"replayEntry": map[string]any{
		"__typename": "ReplayEntryHttpOnePipeline",
		"id":         id, "error": nil, "httpEntries": httpEntries,
	}}
}

func newPipelineEnv(t *testing.T) *testutil.MCPTestEnv {
	t.Helper()
	env := testutil.NewMCPTestEnv(t, func(s *mcp.Server, c *caido.Client) {
		tools.RegisterRacePipelineTool(s, c)
	})
	env.Mock.ValidateAgainstSchema()
	return env
}

const raceRaw = "POST /redeem HTTP/1.1\r\nHost: shop.test\r\nContent-Length: 0\r\n\r\n"

// strategyOf digs settings.strategy out of the recorded create input.
func strategyOf(t *testing.T, env *testutil.MCPTestEnv) map[string]any {
	t.Helper()
	input, _ := env.Mock.LastVariables("CreateReplayPipelineSession")["input"].(map[string]any)
	settings, _ := input["settings"].(map[string]any)
	strategy, _ := settings["strategy"].(map[string]any)
	if strategy == nil {
		t.Fatalf("no strategy in create input %v", input)
	}
	return strategy
}

func TestRacePipeline_SinglePacketDefault(t *testing.T) {
	env := newPipelineEnv(t)
	env.Mock.On("CreateReplayPipelineSession", pipelineCreated("ps-1"))
	env.Mock.On("StartReplayPipelineTask", pipelineStarted("pe-1"))
	// First poll: nothing resolved yet; second: both done.
	env.Mock.On("GetReplayPipelineEntry", pipelineEntry("pe-1"))
	env.Mock.On("GetReplayPipelineEntry", pipelineEntry("pe-1",
		pipelineHTTPEntry("h1", 200, "redeemed"),
		pipelineHTTPEntry("h2", 409, "already used"),
	))

	result := env.CallTool(t, "caido_race_pipeline", map[string]any{
		"host": "shop.test",
		"requests": []map[string]any{
			{"label": "a", "raw": raceRaw}, {"raw": raceRaw},
		},
	})
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	out := testutil.UnmarshalToolResult[tools.RacePipelineOutput](t, result)
	if out.SessionID != "ps-1" || out.EntryID != "pe-1" || !out.Complete {
		t.Fatalf("unexpected run header %+v", out)
	}
	if out.Strategy != "single-packet" {
		t.Fatalf("want default strategy single-packet, got %q", out.Strategy)
	}
	if out.StatusCounts["200"] != 1 || out.StatusCounts["409"] != 1 {
		t.Fatalf("want statusCounts 200:1 409:1, got %v", out.StatusCounts)
	}
	if out.Results[0].Label != "a" || out.Results[1].Label != "req-2" {
		t.Fatalf("labels not mapped by position: %+v", out.Results)
	}
	if out.Results[0].Body != "redeemed" || out.Results[1].RequestID != "req-h2" {
		t.Fatalf("result details not mapped: %+v", out.Results)
	}

	spa, _ := strategyOf(t, env)["singlePacketAttack"].(map[string]any)
	if spa["convertToHttp2"] != true || spa["failureBehavior"] != "ABORT_ON_PARTIAL" {
		t.Fatalf("want singlePacketAttack{convertToHttp2:true, ABORT_ON_PARTIAL}, got %v", spa)
	}
	input, _ := env.Mock.LastVariables("CreateReplayPipelineSession")["input"].(map[string]any)
	sources, _ := input["requestSources"].([]any)
	if len(sources) != 2 {
		t.Fatalf("want 2 request sources, got %d", len(sources))
	}
	if _, ok := input["collectionId"]; ok {
		t.Fatal("collectionId must be omitted when not given")
	}
}

func TestRacePipeline_LastByteFiledAndNamed(t *testing.T) {
	env := newPipelineEnv(t)
	env.Mock.On("CreateReplayPipelineSession", pipelineCreated("ps-2"))
	env.Mock.On("RenameReplaySession", testutil.RenameReplaySessionResponse("ps-2", "Race - coupon"))
	env.Mock.On("StartReplayPipelineTask", pipelineStarted("pe-2"))
	env.Mock.On("GetReplayPipelineEntry", pipelineEntry("pe-2", pipelineHTTPEntry("h1", 200, "ok")))

	result := env.CallTool(t, "caido_race_pipeline", map[string]any{
		"host": "shop.test", "tls": false,
		"requests":      []map[string]any{{"raw": raceRaw}},
		"strategy":      "last-byte",
		"fireOnPartial": true,
		"collectionId":  "col-3",
		"name":          "Race - coupon",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	lbs, _ := strategyOf(t, env)["lastByteSynchronization"].(map[string]any)
	if lbs["failureBehavior"] != "FIRE_ON_PARTIAL" {
		t.Fatalf("want lastByteSynchronization FIRE_ON_PARTIAL, got %v", lbs)
	}
	input, _ := env.Mock.LastVariables("CreateReplayPipelineSession")["input"].(map[string]any)
	if input["collectionId"] != "col-3" {
		t.Fatalf("want collectionId col-3, got %v", input["collectionId"])
	}
	src, _ := input["requestSources"].([]any)
	raw, _ := src[0].(map[string]any)["raw"].(map[string]any)
	ci, _ := raw["connectionInfo"].(map[string]any)
	if ci["isTLS"] != false || ci["port"] != float64(80) {
		t.Fatalf("want plain-HTTP port 80 connection, got %v", ci)
	}
	if got := env.Mock.LastVariables("RenameReplaySession")["name"]; got != "Race - coupon" {
		t.Fatalf("want rename to %q, got %v", "Race - coupon", got)
	}
}

func TestRacePipeline_Sequential(t *testing.T) {
	env := newPipelineEnv(t)
	env.Mock.On("CreateReplayPipelineSession", pipelineCreated("ps-3"))
	env.Mock.On("StartReplayPipelineTask", pipelineStarted("pe-3"))
	env.Mock.On("GetReplayPipelineEntry", pipelineEntry("pe-3", pipelineHTTPEntry("h1", 200, "")))

	result := env.CallTool(t, "caido_race_pipeline", map[string]any{
		"host":           "shop.test",
		"requests":       []map[string]any{{"raw": raceRaw}},
		"strategy":       "sequential",
		"abortOnFailure": true,
	})
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	seq, _ := strategyOf(t, env)["sequential"].(map[string]any)
	if seq["abortOnFailure"] != true {
		t.Fatalf("want sequential abortOnFailure true, got %v", seq)
	}
}

func TestRacePipeline_TimeoutReportsPending(t *testing.T) {
	env := newPipelineEnv(t)
	env.Mock.On("CreateReplayPipelineSession", pipelineCreated("ps-4"))
	env.Mock.On("StartReplayPipelineTask", pipelineStarted("pe-4"))
	env.Mock.On("GetReplayPipelineEntry", pipelineEntry("pe-4",
		pipelineHTTPEntry("h1", 200, "ok"), pipelineHTTPEntry("h2", 0, ""),
	))

	result := env.CallTool(t, "caido_race_pipeline", map[string]any{
		"host":       "shop.test",
		"requests":   []map[string]any{{"raw": raceRaw}, {"raw": raceRaw}},
		"timeoutSec": 1,
	})
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	out := testutil.UnmarshalToolResult[tools.RacePipelineOutput](t, result)
	if out.Complete {
		t.Fatal("want complete=false when a request never resolves")
	}
	if !out.Results[1].Pending || out.StatusCounts["pending"] != 1 {
		t.Fatalf("want second result pending, got %+v counts %v", out.Results[1], out.StatusCounts)
	}
}

func TestRacePipeline_Errors(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
		mock func(env *testutil.MCPTestEnv)
	}{
		{
			name: "no requests",
			args: map[string]any{"host": "shop.test", "requests": []map[string]any{}},
		},
		{
			name: "unknown strategy",
			args: map[string]any{
				"host": "shop.test", "strategy": "yolo",
				"requests": []map[string]any{{"raw": raceRaw}},
			},
		},
		{
			name: "create user error surfaces",
			args: map[string]any{"host": "shop.test", "requests": []map[string]any{{"raw": raceRaw}}},
			mock: func(env *testutil.MCPTestEnv) {
				env.Mock.On("CreateReplayPipelineSession", map[string]any{
					"createReplayPipelineHttpOneSession": map[string]any{
						"session": nil,
						"error":   map[string]any{"__typename": "OtherUserError", "code": "boom"},
					},
				})
			},
		},
		{
			name: "start task in progress surfaces",
			args: map[string]any{"host": "shop.test", "requests": []map[string]any{{"raw": raceRaw}}},
			mock: func(env *testutil.MCPTestEnv) {
				env.Mock.On("CreateReplayPipelineSession", pipelineCreated("ps-5"))
				env.Mock.On("StartReplayPipelineTask", map[string]any{
					"startReplayTask": map[string]any{
						"task":  nil,
						"error": map[string]any{"__typename": "TaskInProgressUserError", "code": "busy"},
					},
				})
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newPipelineEnv(t)
			if tt.mock != nil {
				tt.mock(env)
			}
			result := env.CallTool(t, "caido_race_pipeline", tt.args)
			if !result.IsError {
				t.Fatalf("expected error result, got %+v", result.Content)
			}
		})
	}
}
