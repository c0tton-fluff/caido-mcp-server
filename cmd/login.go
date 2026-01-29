package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/c0tton-fluff/caido-mcp-server/internal/auth"
	"github.com/c0tton-fluff/caido-mcp-server/internal/caido"
	"github.com/spf13/cobra"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with Caido",
	Long: `Authenticate with Caido using the OAuth device flow.

This command initiates an OAuth authentication flow:
1. Opens your browser to the Caido authentication page
2. Displays a code to enter in the browser
3. Waits for you to complete authentication
4. Saves the token to ~/.caido-mcp/token.json for later use by the 'serve' command.`,
	RunE: runLogin,
}

func init() {
	rootCmd.AddCommand(loginCmd)
}

func runLogin(cmd *cobra.Command, args []string) error {
	caidoURL, err := getCaidoURL(cmd)
	if err != nil {
		return err
	}

	// Create Caido client
	client := caido.NewClient(caidoURL)

	// Create authenticator
	authenticator, err := auth.NewAuthenticator(client)
	if err != nil {
		return fmt.Errorf("failed to create authenticator: %w", err)
	}

	// Start authentication with a reasonable timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Run the OAuth flow
	token, err := authenticator.EnsureAuthenticated(ctx)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	// Verify it works
	client.SetToken(token)
	_, err = client.ListRequests(ctx, caido.ListRequestsOptions{First: 1})
	if err != nil {
		return fmt.Errorf("token verification failed: %w", err)
	}

	fmt.Println()
	fmt.Println("You can now use 'caido-mcp serve' to start the MCP server.")

	return nil
}
