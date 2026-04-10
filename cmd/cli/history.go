package main

import (
	"context"
	"fmt"
	"time"

	"github.com/c0tton-fluff/caido-mcp-server/internal/httputil"
	caido "github.com/caido-community/sdk-go"
	"github.com/spf13/cobra"
)

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "List proxy history",
	Long: `List HTTP requests from Caido proxy history.

Examples:
  caido history
  caido history -f 'req.host.eq:"target.com"' -n 20
  caido history --after CURSOR`,
	RunE: runHistory,
}

func init() {
	historyCmd.Flags().StringP(
		"filter", "f", "", "HTTPQL filter query",
	)
	historyCmd.Flags().IntP("limit", "n", 20, "Max results (max 100)")
	historyCmd.Flags().String("after", "", "Pagination cursor")
}

func runHistory(cmd *cobra.Command, args []string) error {
	client, err := newClient(cmd)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(
		context.Background(), 15*time.Second,
	)
	defer cancel()

	filter, _ := cmd.Flags().GetString("filter")
	limit, _ := cmd.Flags().GetInt("limit")
	after, _ := cmd.Flags().GetString("after")

	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	opts := &caido.ListRequestsOptions{First: &limit}
	if filter != "" {
		opts.Filter = &filter
	}
	if after != "" {
		opts.After = &after
	}

	resp, err := client.Requests.List(ctx, opts)
	if err != nil {
		return fmt.Errorf("list requests: %w", err)
	}

	conn := resp.Requests

	// Table header
	fmt.Printf(
		"%-8s %-7s %-6s %s\n", "ID", "METHOD", "STATUS", "URL",
	)

	for _, edge := range conn.Edges {
		r := edge.Node
		url := httputil.BuildURL(r.IsTls, r.Host, r.Port, r.Path, r.Query)
		// Truncate long URLs for table display
		if len(url) > 100 {
			url = url[:97] + "..."
		}
		status := "-"
		if r.Response != nil {
			status = fmt.Sprintf("%d", r.Response.StatusCode)
		}
		fmt.Printf(
			"%-8s %-7s %-6s %s\n",
			r.Id, r.Method, status, url,
		)
	}

	if conn.PageInfo.HasNextPage &&
		conn.PageInfo.EndCursor != nil {
		fmt.Printf("\n--after %s\n", *conn.PageInfo.EndCursor)
	}

	return nil
}
