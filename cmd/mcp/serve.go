package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/c0tton-fluff/caido-mcp-server/internal/auth"
	"github.com/c0tton-fluff/caido-mcp-server/internal/resources"
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

	authMode := "OAuth (stored login)"
	token, tokenSource := staticToken()
	if token != "" {
		authMode = "static access token (" + tokenSource + ")"
		client, err = caido.NewClient(caido.Options{
			URL:  caidoURL,
			Auth: caido.PATAuth(token),
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

	toolCount := tools.RegisterAll(server, client)

	// Resources (read-only data for agent context)
	resourceCount := resources.RegisterAll(server, client)

	logStartup(caidoURL, authMode, toolCount, resourceCount)

	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}

// staticToken returns the Caido GraphQL access token from the environment,
// preferring CAIDO_ACCESS_TOKEN and falling back to the legacy CAIDO_PAT. The second
// return value names the variable the token was read from ("" if neither set).
func staticToken() (token, source string) {
	if t := os.Getenv("CAIDO_ACCESS_TOKEN"); t != "" {
		return t, "CAIDO_ACCESS_TOKEN"
	}
	if t := os.Getenv("CAIDO_PAT"); t != "" {
		fmt.Fprintln(os.Stderr,
			"warning: CAIDO_PAT is deprecated; rename it to CAIDO_ACCESS_TOKEN")
		return t, "CAIDO_PAT"
	}
	return "", ""
}

// logStartup prints a startup banner to stderr. It must never write to
// stdout: stdout is the MCP JSON-RPC channel and any extra bytes there
// would corrupt the protocol stream.
func logStartup(caidoURL, authMode string, toolCount, resourceCount int) {
	fmt.Fprintf(os.Stderr, "caido-mcp-server %s\n", version)
	fmt.Fprintf(os.Stderr, "  Caido URL:  %s\n", caidoURL)
	fmt.Fprintf(os.Stderr, "  Auth:       %s\n", authMode)
	fmt.Fprintf(os.Stderr, "  Registered: %d tools, %d resources\n",
		toolCount, resourceCount)
	fmt.Fprintln(os.Stderr,
		"  Transport:  stdio (ready, waiting for MCP client)")
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
