package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check Caido instance health and auth",
	RunE:  runStatus,
}

func runStatus(cmd *cobra.Command, args []string) error {
	tok, err := loadToken()
	if err != nil {
		fmt.Printf("Auth: FAIL - %v\n", err)
		return nil
	}

	expires := tok.ExpiresAt.Format(time.RFC3339)
	remaining := time.Until(tok.ExpiresAt).Round(time.Minute)
	if remaining > 0 {
		fmt.Printf("Auth: OK (expires %s, %v remaining)\n",
			expires, remaining,
		)
	} else {
		fmt.Printf("Auth: EXPIRED (was %s)\n", expires)
	}

	client, err := newClient(cmd)
	if err != nil {
		fmt.Printf("Instance: FAIL - %v\n", err)
		return nil
	}

	ctx, cancel := context.WithTimeout(
		context.Background(), 5*time.Second,
	)
	defer cancel()

	resp, err := client.Instance.GetRuntime(ctx)
	if err != nil {
		fmt.Printf("Instance: FAIL - %v\n", err)
		return nil
	}

	r := resp.Runtime
	fmt.Printf("Instance: OK (v%s, %s)\n", r.Version, r.Platform)
	return nil
}
