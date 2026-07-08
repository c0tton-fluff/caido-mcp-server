package resources_test

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestReadProjectResource(t *testing.T) {
	env := newResourceTestEnv(t)
	env.Mock.On("GetRuntime", runtimeResponse("1.2.3", "darwin"))
	env.Mock.On("GetCurrentProject", currentProjectResponse("proj-1", "My Project", "READY"))

	result, err := env.Client.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "caido://project",
	})
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if len(result.Contents) == 0 {
		t.Fatal("expected content")
	}

	text := result.Contents[0].Text
	for _, want := range []string{"My Project", "1.2.3", "darwin", "proj-1", "READY"} {
		if !strings.Contains(text, want) {
			t.Errorf("expected output to contain %q, got:\n%s", want, text)
		}
	}
}

func TestReadProjectResourceNoProjectSelected(t *testing.T) {
	env := newResourceTestEnv(t)
	env.Mock.On("GetRuntime", runtimeResponse("1.2.3", "darwin"))
	env.Mock.On("GetCurrentProject", map[string]any{
		"currentProject": nil,
	})

	result, err := env.Client.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "caido://project",
	})
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if len(result.Contents) == 0 {
		t.Fatal("expected content")
	}

	text := result.Contents[0].Text
	if !strings.Contains(text, "no project selected") {
		t.Errorf("expected 'no project selected' note, got:\n%s", text)
	}
	if !strings.Contains(text, "1.2.3") {
		t.Errorf("expected runtime version preserved, got:\n%s", text)
	}
	if !strings.Contains(text, "darwin") {
		t.Errorf("expected runtime platform preserved, got:\n%s", text)
	}
}

func TestReadProjectResourceListable(t *testing.T) {
	env := newResourceTestEnv(t)

	var uris []string
	for res, err := range env.Client.Resources(context.Background(), nil) {
		if err != nil {
			t.Fatalf("Resources: %v", err)
		}
		uris = append(uris, res.URI)
	}

	found := false
	for _, uri := range uris {
		if uri == "caido://project" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("missing resource caido://project, got: %v", uris)
	}
}

func runtimeResponse(version, platform string) map[string]any {
	return map[string]any{
		"runtime": map[string]any{
			"version":  version,
			"platform": platform,
			"logs":     "",
		},
	}
}

func currentProjectResponse(id, name, status string) map[string]any {
	return map[string]any{
		"currentProject": map[string]any{
			"project": map[string]any{
				"id":        id,
				"name":      name,
				"path":      "/tmp/" + id,
				"size":      0,
				"status":    status,
				"temporary": false,
				"createdAt": "2026-01-01T00:00:00Z",
				"updatedAt": "2026-01-01T00:00:00Z",
				"version":   "1",
			},
			"readOnly": false,
		},
	}
}
