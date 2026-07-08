package resources_test

import (
	"context"
	"strings"
	"testing"

	"github.com/c0tton-fluff/caido-mcp-server/internal/resources"
	"github.com/c0tton-fluff/caido-mcp-server/internal/testutil"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type resourceTestEnv struct {
	Mock   *testutil.MockHandler
	Client *mcp.ClientSession
	cancel context.CancelFunc
}

func newResourceTestEnv(t *testing.T) *resourceTestEnv {
	t.Helper()
	env := testutil.NewTestEnv(t)

	server := mcp.NewServer(
		&mcp.Implementation{Name: "test-server", Version: "0.0.1"},
		nil,
	)
	resources.RegisterAll(server, env.Client)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	go func() {
		_, _ = server.Connect(ctx, serverTransport, nil)
	}()

	mcpClient := mcp.NewClient(
		&mcp.Implementation{Name: "test-client", Version: "0.0.1"},
		nil,
	)
	session, err := mcpClient.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("mcp client connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	return &resourceTestEnv{
		Mock:   env.Mock,
		Client: session,
		cancel: cancel,
	}
}

func TestResourceTemplatesListable(t *testing.T) {
	env := newResourceTestEnv(t)

	var templates []string
	for tmpl, err := range env.Client.ResourceTemplates(context.Background(), nil) {
		if err != nil {
			t.Fatalf("ResourceTemplates: %v", err)
		}
		templates = append(templates, tmpl.URITemplate)
	}

	want := []string{
		"caido://requests/{id}",
		"caido://replay-sessions/{id}",
	}
	for _, w := range want {
		found := false
		for _, tmpl := range templates {
			if tmpl == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing resource template %q", w)
		}
	}
}

func TestResourcesListable(t *testing.T) {
	env := newResourceTestEnv(t)

	var uris []string
	for res, err := range env.Client.Resources(context.Background(), nil) {
		if err != nil {
			t.Fatalf("Resources: %v", err)
		}
		uris = append(uris, res.URI)
	}

	want := []string{
		"caido://sitemap",
		"caido://findings",
	}
	for _, w := range want {
		found := false
		for _, uri := range uris {
			if uri == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing resource %q", w)
		}
	}
}

func TestReadRequestResource(t *testing.T) {
	env := newResourceTestEnv(t)
	env.Mock.On("GetRequest", testutil.GetRequestFullResponse("req-1", "hello world"))

	result, err := env.Client.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "caido://requests/req-1",
	})
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}

	if len(result.Contents) == 0 {
		t.Fatal("expected content")
	}
	text := result.Contents[0].Text
	if !strings.Contains(text, "Request req-1") {
		t.Errorf("expected request ID in output, got: %s", text)
	}
	if !strings.Contains(text, "example.com") {
		t.Errorf("expected host in output, got: %s", text)
	}
}

func TestReadSitemapResource(t *testing.T) {
	env := newResourceTestEnv(t)
	env.Mock.On("ListSitemapRootEntries", sitemapRootResponse())

	result, err := env.Client.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "caido://sitemap",
	})
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}

	if len(result.Contents) == 0 {
		t.Fatal("expected content")
	}
	text := result.Contents[0].Text
	if !strings.Contains(text, "example.com") {
		t.Errorf("expected domain in output, got: %s", text)
	}
}

func TestReadFindingsResource(t *testing.T) {
	env := newResourceTestEnv(t)
	env.Mock.On("ListFindings", findingsResponse())

	result, err := env.Client.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "caido://findings",
	})
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}

	if len(result.Contents) == 0 {
		t.Fatal("expected content")
	}
	text := result.Contents[0].Text
	if !strings.Contains(text, "XSS in search") {
		t.Errorf("expected finding title in output, got: %s", text)
	}
}

func TestReadReplaySessionResource(t *testing.T) {
	env := newResourceTestEnv(t)
	env.Mock.On("GetReplaySession", testutil.GetReplaySessionResponse("sess-1", "entry-1"))

	result, err := env.Client.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "caido://replay-sessions/sess-1",
	})
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}

	if len(result.Contents) == 0 {
		t.Fatal("expected content")
	}
	text := result.Contents[0].Text
	if !strings.Contains(text, "test-session") {
		t.Errorf("expected session name in output, got: %s", text)
	}
}

func sitemapRootResponse() map[string]any {
	return map[string]any{
		"sitemapRootEntries": map[string]any{
			"edges": []any{
				map[string]any{
					"node": map[string]any{
						"id":             "site-1",
						"label":          "example.com",
						"kind":           "DOMAIN",
						"hasDescendants": true,
					},
				},
			},
			"pageInfo": map[string]any{
				"hasNextPage": false,
			},
		},
	}
}

func findingsResponse() map[string]any {
	return map[string]any{
		"findings": map[string]any{
			"edges": []any{
				map[string]any{
					"node": map[string]any{
						"id":          "finding-1",
						"title":       "XSS in search",
						"host":        "example.com",
						"path":        "/search",
						"reporter":    "manual",
						"createdAt":   int64(1714900000000),
						"description": nil,
						"request": map[string]any{
							"id": "req-99",
						},
					},
				},
			},
			"pageInfo": map[string]any{
				"hasNextPage": false,
				"endCursor":   nil,
			},
		},
	}
}

// Unused - just ensures the registration function signature is compatible.
var _ = func(s *mcp.Server, c *caido.Client) { resources.RegisterAll(s, c) }
