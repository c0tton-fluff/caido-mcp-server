package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "caido-cli",
	Short: "Pentest HTTP client through Caido proxy",
	Long:  "CLI for Caido proxy -- send requests, browse history, encode/decode.",
}

func main() {
	rootCmd.PersistentFlags().StringP(
		"url", "u", "",
		"Caido instance URL (or set CAIDO_URL)",
	)
	rootCmd.PersistentFlags().IntP(
		"body-limit", "b", 2000,
		"Response body byte limit",
	)

	rootCmd.AddCommand(sendCmd)
	rootCmd.AddCommand(rawCmd)
	rootCmd.AddCommand(batchCmd)
	rootCmd.AddCommand(historyCmd)
	rootCmd.AddCommand(requestCmd)
	rootCmd.AddCommand(encodeCmd)
	rootCmd.AddCommand(decodeCmd)
	rootCmd.AddCommand(statusCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
