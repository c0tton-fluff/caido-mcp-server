package tools

import "github.com/modelcontextprotocol/go-sdk/mcp"

// boolPtr returns a pointer to b, for the *bool annotation hint fields.
func boolPtr(b bool) *bool { return &b }

// readOnlyAnn annotates a tool that only reads or inspects state: no side
// effects and no external network calls (queries the local Caido instance).
func readOnlyAnn() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint:  true,
		OpenWorldHint: boolPtr(false),
	}
}

// writeAnn annotates a mutating tool.
//
//   - destructive: the call may irreversibly remove or overwrite data.
//   - idempotent:  repeating the call with the same args has the same effect.
//   - openWorld:   the call contacts external targets, not just the local
//     Caido instance (e.g. sending an HTTP request to a scanned host).
func writeAnn(destructive, idempotent, openWorld bool) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		DestructiveHint: boolPtr(destructive),
		IdempotentHint:  idempotent,
		OpenWorldHint:   boolPtr(openWorld),
	}
}
