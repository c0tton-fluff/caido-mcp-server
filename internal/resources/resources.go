// Package resources exposes read-only Caido state (proxy history, replay
// sessions, sitemap, findings, scopes, current project) to MCP clients as
// resources, so agents can inspect context without spending tool calls.
package resources

import (
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterAll registers every resource on the server and returns the
// number of resources registered.
func RegisterAll(server *mcp.Server, client *caido.Client) int {
	registers := []func(*mcp.Server, *caido.Client){
		registerRequestResource,
		registerReplaySessionResource,
		registerSitemapResource,
		registerFindingsResource,
		registerScopesResource,
		registerProjectResource,
	}
	for _, register := range registers {
		register(server, client)
	}
	return len(registers)
}
