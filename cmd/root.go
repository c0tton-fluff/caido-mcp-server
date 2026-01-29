package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

const version = "1.0.0"

var rootCmd = &cobra.Command{
	Use:     "caido-mcp-server",
	Short:   "MCP server for Caido proxy",
	Long:    `A Model Context Protocol (MCP) server that provides access to Caido proxy history.`,
	Version: version,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringP("url", "u", "", "Caido instance URL (or set CAIDO_URL env var)")
}

// getCaidoURL returns the Caido URL from flag or environment variable
func getCaidoURL(cmd *cobra.Command) (string, error) {
	url, _ := cmd.Flags().GetString("url")
	if url == "" {
		url = os.Getenv("CAIDO_URL")
	}
	if url == "" {
		return "", fmt.Errorf("Caido URL is required. Set --url flag or CAIDO_URL environment variable")
	}
	return url, nil
}
