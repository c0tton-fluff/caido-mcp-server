package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var version = "dev"

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
		return "", fmt.Errorf("caido URL is required: set --url flag or CAIDO_URL env var")
	}
	return url, nil
}
