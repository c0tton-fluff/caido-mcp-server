package main

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestProtocolVersionNegotiation guards MCP 2026-07-28 conformance: the server,
// constructed exactly as serve.go constructs it (nil ServerOptions, so it
// defaults to the SDK's latestProtocolVersion), must negotiate protocol version
// 2026-07-28 with a modern client. A future go-sdk downgrade or an accidental
// version pin in serve.go would fail this test.
func TestProtocolVersionNegotiation(t *testing.T) {
	ctx := context.Background()

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "caido-mcp-server",
		Version: "test",
	}, nil)

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
}
