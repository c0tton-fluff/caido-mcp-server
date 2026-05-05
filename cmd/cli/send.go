package main

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var sendCmd = &cobra.Command{
	Use:   "send METHOD URL",
	Short: "Send HTTP request (structured)",
	Long: `Send an HTTP request through Caido's Replay API.

Builds the raw HTTP request from method, URL, headers, and body.
Same interface as 'burp send'.

Examples:
  caido send GET https://target.com/api/users
  caido send POST https://target.com/api/login -j '{"user":"admin","pass":"test"}'
  caido send PUT https://target.com/api/profile -H "Authorization: Bearer tok" -j '{"role":"admin"}'
  caido send DELETE https://target.com/api/users/5 -H "Cookie: session=abc"`,
	Args: cobra.ExactArgs(2),
	RunE: runSend,
}

func init() {
	f := sendCmd.Flags()
	f.StringArrayP("header", "H", nil, "Header (repeatable)")
	f.StringP("data", "d", "", "Request body (prefix @ for file)")
	f.StringP("json", "j", "", "JSON body (prefix @ for file)")
	f.Bool("all-headers", false, "Show all response headers")
}

func runSend(cmd *cobra.Command, args []string) error {
	method := strings.ToUpper(args[0])
	rawURL := args[1]

	headers, _ := cmd.Flags().GetStringArray("header")
	data, _ := cmd.Flags().GetString("data")
	jsonData, _ := cmd.Flags().GetString("json")
	allHeaders, _ := cmd.Flags().GetBool("all-headers")
	bodyLimit, _ := cmd.Flags().GetInt("body-limit")

	body := ""
	if jsonData != "" {
		body = resolveBody(jsonData)
		if !hasHeaderPrefix(headers, "content-type") {
			headers = append(headers, "Content-Type: application/json")
		}
	} else if data != "" {
		body = resolveBody(data)
	}

	raw, host, port, useTLS, err := buildRequest(
		method, rawURL, headers, body,
	)
	if err != nil {
		return err
	}

	client, err := newClient(cmd)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(
		context.Background(), 30*time.Second,
	)
	defer cancel()

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

// buildRequest assembles a raw HTTP request from structured components.
// Returns (rawHTTP, host, port, tls, error).
func buildRequest(
	method, rawURL string, headers []string, body string,
) (string, string, int, bool, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", 0, false, fmt.Errorf("bad URL: %w", err)
	}

	host := u.Hostname()
	if host == "" {
		return "", "", 0, false, fmt.Errorf("no host in URL: %s", rawURL)
	}

	tls := u.Scheme == "https"
	port := 0
	if u.Port() != "" {
		fmt.Sscanf(u.Port(), "%d", &port)
	}
	if port == 0 {
		if tls {
			port = 443
		} else {
			port = 80
		}
	}

	path := u.RequestURI()
	if path == "" {
		path = "/"
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("%s %s HTTP/1.1", method, path))

	if !hasHeaderPrefix(headers, "host") {
		hv := host
		if (tls && port != 443) || (!tls && port != 80) {
			hv = fmt.Sprintf("%s:%d", host, port)
		}
		lines = append(lines, "Host: "+hv)
	}

	if !hasHeaderPrefix(headers, "connection") {
		lines = append(lines, "Connection: close")
	}

	lines = append(lines, headers...)

	if body != "" && !hasHeaderPrefix(headers, "content-length") {
		lines = append(lines,
			fmt.Sprintf("Content-Length: %d", len(body)),
		)
	}

	raw := strings.Join(lines, "\r\n") + "\r\n\r\n"
	if body != "" {
		raw += body
	}

	return raw, host, port, tls, nil
}

func hasHeaderPrefix(headers []string, prefix string) bool {
	prefix = strings.ToLower(prefix)
	for _, h := range headers {
		if idx := strings.Index(h, ":"); idx > 0 {
			if strings.ToLower(strings.TrimSpace(h[:idx])) == prefix {
				return true
			}
		}
	}
	return false
}

func resolveBody(val string) string {
	if strings.HasPrefix(val, "@") {
		data, err := readFileBytes(val[1:])
		if err != nil {
			return val
		}
		return string(data)
	}
	return val
}
