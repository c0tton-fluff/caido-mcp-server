package main

import (
	"context"
	"testing"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestProtocolVersionNegotiation guards MCP 2026-07-28 conformance: the server,
// built by the same newServer serve.go runs (no protocol version pin, so it
// defaults to the SDK's latestProtocolVersion), must negotiate protocol version
// 2026-07-28 with a modern client. A future go-sdk downgrade or an accidental
// version pin in serve.go would fail this test.
func TestProtocolVersionNegotiation(t *testing.T) {
	ctx := context.Background()

	server := newServer(tools.Options{})

	clientTransport, serverTransport := mcp.NewInMemoryTransports()

	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "conformance-probe", Version: "test"}, nil)
	cs, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer func() { _ = cs.Close() }()

	const want = "2026-07-28"
	got := cs.InitializeResult().ProtocolVersion
	if got != want {
		t.Fatalf("negotiated MCP protocol version = %q, want %q", got, want)
	}

	// The server instructions must reach the client in the initialize result.
	if got := cs.InitializeResult().Instructions; got != tools.Instructions {
		t.Fatalf("initialize instructions = %q, want tools.Instructions", got)
	}
}
