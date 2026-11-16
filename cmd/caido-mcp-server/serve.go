package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/auth"
	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/buildinfo"
	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/resources"
	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/tools"
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

// newServer builds the MCP server without tools or resources. It is the
// single construction site, so tests exercise the same options serve runs
// with. Protocol versions are left at the SDK default (latest supported).
func newServer(opts tools.Options) *mcp.Server {
	return mcp.NewServer(
		&mcp.Implementation{
			Name:    "caido-mcp-server",
			Version: buildinfo.Version(),
		},
		&mcp.ServerOptions{Instructions: tools.InstructionsFor(opts)},
	)
}

func init() {
	serveCmd.Flags().Bool("read-only", false,
		"Expose only tools that read state: nothing that sends traffic or "+
			"changes the Caido project (or set CAIDO_MCP_READ_ONLY=1)")
	serveCmd.Flags().Bool("enforce-scope", false,
		"Refuse requests to hosts outside the project scope and withhold the "+
			"tools that change scopes or switch projects (or set CAIDO_MCP_ENFORCE_SCOPE=1)")
	serveCmd.Flags().StringSlice("toolsets", nil,
		"Register only these toolsets, comma separated (or set CAIDO_MCP_TOOLSETS). "+
			"Default: all. Available:\n  "+strings.Join(tools.ToolsetNames(), "\n  "))
	serveCmd.Flags().Bool("compact", false,
		"Leave output schemas out of the advertised tool list, about a third "+
			"of its size (or set CAIDO_MCP_COMPACT=1)")
	rootCmd.AddCommand(serveCmd)
}

// boolOption reports whether a boolean serve option is on: its flag, or its
// environment variable set to a true value (MCP client configs set
// environment variables more readily than arguments). A value that does not
// parse as a boolean is an error rather than a silent "off": for the safety
// restrictions the operator must not get the unrestricted server from a typo.
func boolOption(cmd *cobra.Command, flag, env string) (bool, error) {
	if on, _ := cmd.Flags().GetBool(flag); on {
		return true, nil
	}
	v := os.Getenv(env)
	if v == "" {
		return false, nil
	}
	on, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s=%q is not a boolean (use 1 or 0)", env, v)
	}
	return on, nil
}

// serveOptions gathers the tool options from flags and environment.
func serveOptions(cmd *cobra.Command) (tools.Options, error) {
	var opts tools.Options
	var err error
	if opts.ReadOnly, err = boolOption(cmd, "read-only", "CAIDO_MCP_READ_ONLY"); err != nil {
		return opts, err
	}
	if opts.EnforceScope, err = boolOption(cmd, "enforce-scope", "CAIDO_MCP_ENFORCE_SCOPE"); err != nil {
		return opts, err
	}
	if opts.Compact, err = boolOption(cmd, "compact", "CAIDO_MCP_COMPACT"); err != nil {
		return opts, err
	}
	opts.Toolsets, _ = cmd.Flags().GetStringSlice("toolsets")
	if len(opts.Toolsets) == 0 {
		if v := os.Getenv("CAIDO_MCP_TOOLSETS"); v != "" {
			opts.Toolsets = strings.Split(v, ",")
		}
	}
	return opts, nil
}

func runServe(cmd *cobra.Command, args []string) error {
	caidoURL, err := getCaidoURL(cmd)
	if err != nil {
		return err
	}
	opts, err := serveOptions(cmd)
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

		token, tokenStore, err := getTokenAndStore(ctx, client, caidoURL)
		if err != nil {
			return err
		}
		client.SetAccessToken(token)

		client.SetTokenRefresher(
			makeTokenRefresher(client, tokenStore),
		)
	}

	server := newServer(opts)

	toolCount, err := tools.RegisterAllWith(ctx, server, client, opts)
	if err != nil {
		return err
	}

	// Resources (read-only data for agent context)
	resourceCount := resources.RegisterAll(server, client)

	logStartup(caidoURL, authMode, toolCount, resourceCount, opts)

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
func logStartup(
	caidoURL, authMode string, toolCount, resourceCount int, opts tools.Options,
) {
	fmt.Fprintf(os.Stderr, "caido-mcp-server %s\n", buildinfo.Version())
	fmt.Fprintf(os.Stderr, "  Caido URL:  %s\n", caidoURL)
	fmt.Fprintf(os.Stderr, "  Auth:       %s\n", authMode)
	fmt.Fprintf(os.Stderr, "  Registered: %d tools, %d resources\n",
		toolCount, resourceCount)
	if len(opts.Toolsets) > 0 {
		fmt.Fprintf(os.Stderr, "  Toolsets:   %s\n", strings.Join(opts.Toolsets, ","))
	}
	if opts.Compact {
		fmt.Fprintln(os.Stderr, "  Compact:    output schemas left out of the tool list")
	}
	if opts.ReadOnly {
		fmt.Fprintln(os.Stderr,
			"  Mode:       read-only (tools that send traffic or change the project are not registered)")
	}
	if opts.EnforceScope {
		fmt.Fprintln(os.Stderr,
			"  Scope:      enforced (out-of-scope targets refused; scope and project tools withheld)")
	}
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

// getTokenAndStore retrieves the access token for caidoURL and returns the
// token store for use in auto-refresh.
func getTokenAndStore(
	ctx context.Context,
	client *caido.Client,
	caidoURL string,
) (string, *auth.TokenStore, error) {
	tokenStore, err := auth.NewTokenStore(caidoURL)
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
			"no authentication token found for %s. "+
				"Please run 'caido-mcp-server login --url %s' first",
			tokenStore.ServerURL(), caidoURL,
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
