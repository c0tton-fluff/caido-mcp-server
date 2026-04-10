package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/c0tton-fluff/caido-mcp-server/internal/auth"
	"github.com/c0tton-fluff/caido-mcp-server/internal/tools"
	caido "github.com/caido-community/sdk-go"
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

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	var client *caido.Client

	pat := os.Getenv("CAIDO_PAT")
	if pat != "" {
		client, err = caido.NewClient(caido.Options{
			URL:  caidoURL,
			Auth: caido.PATAuth(pat),
		})
		if err != nil {
			return fmt.Errorf("failed to create client: %w", err)
		}
	} else {
		client, err = caido.NewClient(
			caido.Options{URL: caidoURL},
		)
		if err != nil {
			return fmt.Errorf("failed to create client: %w", err)
		}

		token, tokenStore, err := getTokenAndStore(ctx, client)
		if err != nil {
			return err
		}
		client.SetAccessToken(token)

		client.SetTokenRefresher(
			makeTokenRefresher(client, tokenStore),
		)
	}

	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "caido-mcp-server",
			Version: version,
		},
		nil,
	)

	// HTTP History
	tools.RegisterListRequestsTool(server, client)
	tools.RegisterGetRequestTool(server, client)

	// Automate (Fuzzing)
	tools.RegisterListAutomateSessionsTool(server, client)
	tools.RegisterGetAutomateSessionTool(server, client)
	tools.RegisterGetAutomateEntryTool(server, client)
	tools.RegisterAutomateTaskControlTool(server, client)

	// Replay (Send Requests)
	tools.RegisterSendRequestTool(server, client)
	tools.RegisterBatchSendTool(server, client)
	tools.RegisterListReplaySessionsTool(server, client)
	tools.RegisterGetReplayEntryTool(server, client)

	// Findings
	tools.RegisterListFindingsTool(server, client)
	tools.RegisterCreateFindingTool(server, client)
	tools.RegisterDeleteFindingsTool(server, client)
	tools.RegisterExportFindingsTool(server, client)

	// Sitemap
	tools.RegisterGetSitemapTool(server, client)

	// Scopes
	tools.RegisterListScopesTool(server, client)
	tools.RegisterCreateScopeTool(server, client)

	// Projects
	tools.RegisterListProjectsTool(server, client)
	tools.RegisterSelectProjectTool(server, client)

	// Workflows
	tools.RegisterListWorkflowsTool(server, client)
	tools.RegisterRunWorkflowTool(server, client)
	tools.RegisterToggleWorkflowTool(server, client)

	// Environments
	tools.RegisterListEnvironmentsTool(server, client)
	tools.RegisterSelectEnvironmentTool(server, client)

	// Instance
	tools.RegisterGetInstanceTool(server, client)

	// Intercept
	tools.RegisterInterceptStatusTool(server, client)
	tools.RegisterInterceptControlTool(server, client)
	tools.RegisterListInterceptEntriesTool(server, client)
	tools.RegisterForwardInterceptTool(server, client)
	tools.RegisterDropInterceptTool(server, client)

	// Tamper (Match & Replace)
	tools.RegisterListTamperRulesTool(server, client)
	tools.RegisterCreateTamperRuleTool(server, client)
	tools.RegisterToggleTamperRuleTool(server, client)
	tools.RegisterDeleteTamperRuleTool(server, client)

	// Filters
	tools.RegisterListFiltersTool(server, client)

	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}

// makeTokenRefresher creates the auto-refresh callback.
func makeTokenRefresher(
	client *caido.Client, tokenStore *auth.TokenStore,
) caido.TokenRefreshFunc {
	return func(ctx context.Context) (string, error) {
		stored, err := tokenStore.Load()
		if err != nil || stored == nil {
			return "", nil
		}
		if !tokenStore.IsExpired(stored) {
			return stored.AccessToken, nil
		}
		if stored.RefreshToken == "" {
			return "", fmt.Errorf(
				"token expired, no refresh token",
			)
		}

		refreshed, err := auth.RefreshAndSave(
			ctx, client, tokenStore, stored.RefreshToken,
		)
		if err != nil {
			return "", err
		}
		return refreshed.AccessToken, nil
	}
}

// getTokenAndStore retrieves the access token and returns the
// token store for use in auto-refresh.
func getTokenAndStore(
	ctx context.Context,
	client *caido.Client,
) (string, *auth.TokenStore, error) {
	tokenStore, err := auth.NewTokenStore()
	if err != nil {
		return "", nil, fmt.Errorf(
			"failed to create token store: %w", err,
		)
	}

	storedToken, err := tokenStore.Load()
	if err != nil {
		return "", nil, fmt.Errorf(
			"failed to load token: %w", err,
		)
	}

	if storedToken == nil {
		return "", nil, fmt.Errorf(
			"no authentication token found. " +
				"Please run 'caido-mcp-server login' first",
		)
	}

	if tokenStore.IsExpired(storedToken) {
		if storedToken.RefreshToken == "" {
			return "", nil, fmt.Errorf(
				"token expired and no refresh token. " +
					"Please run 'caido-mcp-server login' again",
			)
		}

		refreshed, err := auth.RefreshAndSave(
			ctx, client, tokenStore, storedToken.RefreshToken,
		)
		if err != nil {
			return "", nil, fmt.Errorf(
				"token expired and refresh failed: %w. "+
					"Please run 'caido-mcp-server login' again",
				err,
			)
		}
		storedToken = refreshed
	}

	return storedToken.AccessToken, tokenStore, nil
}
