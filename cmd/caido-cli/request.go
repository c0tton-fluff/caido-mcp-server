package main

import (
	"context"
	"fmt"
	"time"

	"github.com/c0tton-fluff/caido-mcp-server/internal/httputil"
	"github.com/spf13/cobra"
)

var requestCmd = &cobra.Command{
	Use:   "request ID",
	Short: "Get full request/response by ID",
	Long: `Fetch a request from proxy history and display full details.

Examples:
  caido request 12345
  caido request 12345 -b 5000`,
	Args: cobra.ExactArgs(1),
	RunE: runRequest,
}

func runRequest(cmd *cobra.Command, args []string) error {
	id := args[0]

	client, err := newClient(cmd)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(
		context.Background(), 15*time.Second,
	)
	defer cancel()

	bodyLimit, _ := cmd.Flags().GetInt("body-limit")

	resp, err := client.Requests.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("get request %s: %w", id, err)
	}

	r := resp.Request
	if r == nil {
		return fmt.Errorf("request %s not found", id)
	}

	// Print request
	reqParsed := httputil.ParseBase64(r.Raw, true, true, 0, bodyLimit)
	fmt.Println(fmtReq(reqParsed))

	// Separator
	fmt.Println("\n---")

	// Print response
	if r.Response != nil {
		respParsed := httputil.ParseBase64(
			r.Response.Raw, true, true, 0, bodyLimit,
		)
		fmt.Println(fmtResp(respParsed, false))
	} else {
		fmt.Println("(no response)")
	}

	return nil
}
