package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/c0tton-fluff/caido-mcp-server/internal/auth"
	"github.com/c0tton-fluff/caido-mcp-server/internal/caido"
	"github.com/c0tton-fluff/caido-mcp-server/internal/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the MCP server",
	Long: `Start the MCP server for Caido.

This command starts an MCP server that communicates via stdio.
It requires authentication via 'caido-mcp-server login' first.`,
	RunE: runServe,
}

func init() {
	rootCmd.AddCommand(serveCmd)
}

func runServe(cmd *cobra.Command, args []string) error {
	caidoURL, err := getCaidoURL(cmd)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	// Create Caido client
	client := caido.NewClient(caidoURL)

	// Get token
	token, err := getToken(ctx, client)
	if err != nil {
		return err
	}
	client.SetToken(token)

	// Create MCP server
	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "caido-mcp-server",
			Version: version,
		},
		nil,
	)

	// Register tools - HTTP History
	tools.RegisterListRequestsTool(server, client)
	tools.RegisterGetRequestTool(server, client)

	// Register tools - Automate (Fuzzing)
	tools.RegisterListAutomateSessionsTool(server, client)
	tools.RegisterGetAutomateSessionTool(server, client)
	tools.RegisterGetAutomateEntryTool(server, client)

	// Register tools - Replay (Send Requests)
	tools.RegisterSendRequestTool(server, client)
	tools.RegisterListReplaySessionsTool(server, client)
	tools.RegisterGetReplayEntryTool(server, client)

	// Register tools - Findings
	tools.RegisterListFindingsTool(server, client)
	tools.RegisterCreateFindingTool(server, client)

	// Register tools - Sitemap
	tools.RegisterGetSitemapTool(server, client)

	// Register tools - Scopes
	tools.RegisterListScopesTool(server, client)
	tools.RegisterCreateScopeTool(server, client)

	// Run the server with stdio transport
	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}

// getToken retrieves the access token from stored file
func getToken(ctx context.Context, client *caido.Client) (string, error) {
	tokenStore, err := auth.NewTokenStore()
	if err != nil {
		return "", fmt.Errorf("failed to create token store: %w", err)
	}

	storedToken, err := tokenStore.Load()
	if err != nil {
		return "", fmt.Errorf("failed to load token: %w", err)
	}

	if storedToken == nil {
		return "", fmt.Errorf("no authentication token found. Please run 'caido-mcp-server login' first")
	}

	// Check if token is expired and needs refresh
	if tokenStore.IsExpired(storedToken) {
		if storedToken.RefreshToken == "" {
			return "", fmt.Errorf("token expired and no refresh token available. Please run 'caido-mcp-server login' again")
		}

		// Try to refresh
		newToken, err := client.RefreshToken(ctx, storedToken.RefreshToken)
		if err != nil {
			return "", fmt.Errorf("token expired and refresh failed: %w. Please run 'caido-mcp-server login' again", err)
		}

		// Save the new token
		storedToken = &auth.StoredToken{
			AccessToken:  newToken.AccessToken,
			RefreshToken: newToken.RefreshToken,
			ExpiresAt:    newToken.ExpiresAt,
		}
		if err := tokenStore.Save(storedToken); err != nil {
			// Log to stderr but don't fail
			fmt.Fprintf(os.Stderr, "Warning: failed to save refreshed token: %v\n", err)
		}
	}

	return storedToken.AccessToken, nil
}
