package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/c0tton-fluff/caido-mcp-server/internal/httputil"
	"github.com/spf13/cobra"
)

var rawCmd = &cobra.Command{
	Use:   "raw [RAW]",
	Short: "Send raw HTTP request via Replay API",
	Long: `Send a raw HTTP request through Caido's Replay API.

Accepts raw HTTP as argument, from file (-f), or stdin.

Examples:
  caido raw 'GET /api/users HTTP/1.1\r\nHost: target.com\r\n\r\n'
  caido raw -f request.txt
  caido raw -f request.txt --host target.com --port 8443
  echo -n 'GET / HTTP/1.1\r\nHost: example.com\r\n\r\n' | caido raw -`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRaw,
}

func init() {
	f := rawCmd.Flags()
	f.StringP("file", "f", "", "Read request from file")
	f.String("host", "", "Override target host")
	f.Int("port", 0, "Override target port")
	f.Bool("tls", true, "Use TLS (default true)")
	f.Bool("no-tls", false, "Disable TLS")
	f.Bool("all-headers", false, "Show all response headers")
}

func runRaw(cmd *cobra.Command, args []string) error {
	var raw string

	file, _ := cmd.Flags().GetString("file")
	if file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("read file: %w", err)
		}
		raw = string(data)
	} else if len(args) > 0 && args[0] != "-" {
		raw = args[0]
	} else {
		data, err := readStdin()
		if err != nil {
			return fmt.Errorf(
				"no input: pipe a request, use -f FILE, or pass as arg",
			)
		}
		raw = string(data)
	}

	if raw == "" {
		return fmt.Errorf("empty request")
	}

	raw = httputil.NormalizeCRLF(raw)

	host, _ := cmd.Flags().GetString("host")
	if host == "" {
		host = httputil.ParseHostHeader(raw)
	}
	if host == "" {
		return fmt.Errorf(
			"no host: provide --host or include Host header",
		)
	}

	port, _ := cmd.Flags().GetInt("port")
	if h, p, err := net.SplitHostPort(host); err == nil {
		host = h
		if port == 0 {
			if pv, e := strconv.Atoi(p); e == nil {
				port = pv
			}
		}
	}

	noTLS, _ := cmd.Flags().GetBool("no-tls")
	useTLS := !noTLS
	if cmd.Flags().Changed("tls") {
		useTLS, _ = cmd.Flags().GetBool("tls")
	}

	if port == 0 {
		if useTLS {
			port = 443
		} else {
			port = 80
		}
	}

	client, err := newClient(cmd)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(
		context.Background(), 30*time.Second,
	)
	defer cancel()

	bodyLimit, _ := cmd.Flags().GetInt("body-limit")
	allHeaders, _ := cmd.Flags().GetBool("all-headers")

	output, err := sendReplay(
		ctx, client, raw, host, port, useTLS,
		bodyLimit, allHeaders,
	)
	if err != nil {
		return err
	}
	fmt.Print(output)
	return nil
}

func readStdin() ([]byte, error) {
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) != 0 {
		return nil, fmt.Errorf("no stdin")
	}
	return io.ReadAll(os.Stdin)
}

func readFileBytes(path string) ([]byte, error) {
	return os.ReadFile(path)
}
