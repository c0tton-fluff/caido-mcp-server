// Package auth handles Caido authentication: the OAuth device/websocket
// login flow, token storage on disk, expiry checks, and automatic token
// refresh for long-running MCP sessions.
//
// Tokens are stored per Caido instance under ~/.caido-mcp/tokens/, keyed by
// the canonical instance URL, so switching instances does not evict the
// previous login.
package auth
