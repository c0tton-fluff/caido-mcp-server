package main

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"strings"
	"time"

	"github.com/c0tton-fluff/caido-mcp-server/internal/replay"
	"github.com/spf13/cobra"
)

var batchCmd = &cobra.Command{
	Use:   "batch MODE",
	Short: "Parallel HTTP requests through Caido Replay API",
	Long: `Send multiple HTTP requests in parallel through Caido.

Modes:
  sweep   Same endpoint, different tokens (BAC testing)
  fuzz    Same endpoint, vary one parameter
  ep      Different endpoints, same auth
  file    Full batch spec from JSON file

Token format:
  owner=eyJ...         labeled token
  eyJ...               token with auto-label
  noauth               no auth token

Auth modes (--auth):
  bearer               Authorization: Bearer TOKEN (default)
  cookie:NAME          Cookie: NAME=TOKEN
  header:NAME          NAME: TOKEN

Examples:
  caido batch sweep https://target.com/api/profile \
    -t "owner=eyJ1...,cross=eyJ2...,noauth"

  caido batch sweep https://target.com/dashboard \
    --auth cookie:token -t "owner=eyJ1...,cross=eyJ2...,noauth"

  caido batch fuzz "https://target.com/api/search?q=test" \
    -p q -v "test,test',test'--,1 OR 1=1" -H "Authorization: Bearer eyJ..."

  caido batch ep -t eyJ... \
    https://target.com/dashboard \
    https://target.com/admin

  caido batch file batch.json`,
}

var (
	batchSweepCmd = &cobra.Command{
		Use:   "sweep URL",
		Short: "Token sweep (same endpoint, N tokens)",
		Args:  cobra.ExactArgs(1),
		RunE:  runBatchSweep,
	}
	batchFuzzCmd = &cobra.Command{
		Use:   "fuzz URL",
		Short: "Parameter fuzzing (same endpoint, N values)",
		Args:  cobra.ExactArgs(1),
		RunE:  runBatchFuzz,
	}
	batchEpCmd = &cobra.Command{
		Use:   "ep URL...",
		Short: "Endpoint sweep (N URLs, same auth)",
		Args:  cobra.MinimumNArgs(1),
		RunE:  runBatchEp,
	}
	batchFileCmd = &cobra.Command{
		Use:   "file BATCH.json",
		Short: "Full batch spec from JSON file",
		Args:  cobra.ExactArgs(1),
		RunE:  runBatchFile,
	}
)

func init() {
	// Shared flags on parent batch command.
	f := batchCmd.PersistentFlags()
	f.StringP("method", "m", "GET", "HTTP method")
	f.StringArrayP("header", "H", nil, "Header (repeatable)")
	f.StringP("data", "d", "", "Request body")
	f.StringP("json", "j", "", "JSON body (auto-sets Content-Type)")
	f.StringP("tokens", "t", "", "Comma-separated label=token pairs")
	f.StringP("param", "p", "", "Parameter name to fuzz")
	f.StringP("values", "v", "", "Comma-separated fuzz values")
	f.IntP("concurrency", "c", 5, "Parallel session count (max 20)")
	f.String("auth", "bearer", "Auth mode: bearer, cookie:NAME, header:NAME")
	f.Int("timeout", 60, "Overall timeout in seconds")
	f.Bool("verbose", false, "Full response output")
	f.Bool("json-output", false, "JSON output")

	batchCmd.AddCommand(batchSweepCmd)
	batchCmd.AddCommand(batchFuzzCmd)
	batchCmd.AddCommand(batchEpCmd)
	batchCmd.AddCommand(batchFileCmd)
}

// tokenDef is a labeled token for sweep mode.
type tokenDef struct {
	Label string
	Value string
}

func runBatchSweep(cmd *cobra.Command, args []string) error {
	url := args[0]
	method, _ := cmd.Flags().GetString("method")
	hdrs := getHeaders(cmd)
	body := getBody(cmd, hdrs)
	tokensRaw, _ := cmd.Flags().GetString("tokens")
	authMode, _ := cmd.Flags().GetString("auth")

	tokens := parseTokens(tokensRaw)
	if len(tokens) == 0 {
		return fmt.Errorf("sweep requires -t with tokens")
	}

	var reqs []replay.BatchRequest
	for _, tok := range tokens {
		h := copyHeaders(hdrs)
		applyToken(h, tok.Value, authMode)
		raw, host, port, tls, err := buildRawRequest(method, url, h, body)
		if err != nil {
			return fmt.Errorf("build request for %s: %w", tok.Label, err)
		}
		reqs = append(reqs, replay.BatchRequest{
			Label: tok.Label,
			Raw:   raw,
			Host:  host,
			Port:  port,
			TLS:   &tls,
		})
	}

	return executeBatch(cmd, reqs)
}

func runBatchFuzz(cmd *cobra.Command, args []string) error {
	baseURL := args[0]
	method, _ := cmd.Flags().GetString("method")
	hdrs := getHeaders(cmd)
	body := getBody(cmd, hdrs)
	param, _ := cmd.Flags().GetString("param")
	valuesRaw, _ := cmd.Flags().GetString("values")

	if param == "" && !strings.Contains(body, "{{FUZZ}}") {
		return fmt.Errorf("fuzz requires -p PARAM or {{FUZZ}} in body")
	}
	values := splitCSV(valuesRaw)
	if len(values) == 0 {
		return fmt.Errorf("fuzz requires -v with values")
	}

	var reqs []replay.BatchRequest
	for _, v := range values {
		var itemURL, itemBody string
		if body != "" && strings.Contains(body, "{{FUZZ}}") {
			itemBody = strings.ReplaceAll(body, "{{FUZZ}}", v)
			itemURL = baseURL
		} else if param != "" {
			itemURL = replaceParam(baseURL, param, v)
			itemBody = body
		}

		raw, host, port, tls, err := buildRawRequest(
			method, itemURL, copyHeaders(hdrs), itemBody,
		)
		if err != nil {
			return fmt.Errorf("build request for %s: %w", v, err)
		}
		reqs = append(reqs, replay.BatchRequest{
			Label: v,
			Raw:   raw,
			Host:  host,
			Port:  port,
			TLS:   &tls,
		})
	}

	return executeBatch(cmd, reqs)
}

func runBatchEp(cmd *cobra.Command, args []string) error {
	hdrs := getHeaders(cmd)
	body := getBody(cmd, hdrs)
	tokensRaw, _ := cmd.Flags().GetString("tokens")
	authMode, _ := cmd.Flags().GetString("auth")
	method, _ := cmd.Flags().GetString("method")

	tokens := parseTokens(tokensRaw)
	if len(tokens) > 0 && tokens[0].Value != "" {
		applyToken(hdrs, tokens[0].Value, authMode)
	}

	var reqs []replay.BatchRequest
	for _, u := range args {
		raw, host, port, tls, err := buildRawRequest(
			method, u, copyHeaders(hdrs), body,
		)
		if err != nil {
			return fmt.Errorf("build request for %s: %w", u, err)
		}
		reqs = append(reqs, replay.BatchRequest{
			Label: method + " " + u,
			Raw:   raw,
			Host:  host,
			Port:  port,
			TLS:   &tls,
		})
	}

	return executeBatch(cmd, reqs)
}

func runBatchFile(cmd *cobra.Command, args []string) error {
	data, err := os.ReadFile(args[0])
	if err != nil {
		return fmt.Errorf("read %s: %w", args[0], err)
	}

	var spec batchFileSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return fmt.Errorf("parse %s: %w", args[0], err)
	}

	var reqs []replay.BatchRequest
	for i, item := range spec.Requests {
		method := spec.Base.Method
		if item.Method != "" {
			method = item.Method
		}
		if method == "" {
			method = "GET"
		}

		reqURL := spec.Base.URL
		if item.URL != "" {
			reqURL = item.URL
		}

		hdrs := make(map[string]string)
		maps.Copy(hdrs, spec.Base.Headers)
		maps.Copy(hdrs, item.Headers)

		authMode := spec.Base.AuthMode
		if item.AuthMode != "" {
			authMode = item.AuthMode
		}
		applyTokenMap(hdrs, item.Token, authMode)

		body := spec.Base.Body
		if item.Body != "" {
			body = item.Body
		}

		label := item.Label
		if label == "" {
			label = fmt.Sprintf("req-%d", i+1)
		}

		raw, host, port, tls, err := buildRawRequestFromMap(
			method, reqURL, hdrs, body,
		)
		if err != nil {
			return fmt.Errorf("build request %s: %w", label, err)
		}
		reqs = append(reqs, replay.BatchRequest{
			Label: label,
			Raw:   raw,
			Host:  host,
			Port:  port,
			TLS:   &tls,
		})
	}

	return executeBatch(cmd, reqs)
}

// executeBatch runs the batch and prints results.
func executeBatch(
	cmd *cobra.Command, reqs []replay.BatchRequest,
) error {
	client, err := newClient(cmd)
	if err != nil {
		return err
	}

	concurrency, _ := cmd.Flags().GetInt("concurrency")
	bodyLimit, _ := cmd.Flags().GetInt("body-limit")
	timeout, _ := cmd.Flags().GetInt("timeout")
	verbose, _ := cmd.Flags().GetBool("verbose")
	jsonOut, _ := cmd.Flags().GetBool("json-output")

	ctx, cancel := context.WithTimeout(
		context.Background(), time.Duration(timeout)*time.Second,
	)
	defer cancel()

	fmt.Fprintf(os.Stderr,
		"caido batch: %d requests, concurrency %d\n",
		len(reqs), concurrency,
	)

	results := replay.RunBatch(ctx, client, reqs, concurrency, bodyLimit)

	if jsonOut {
		data, _ := json.MarshalIndent(results, "", "  ")
		fmt.Println(string(data))
	} else if verbose {
		printVerbose(results)
	} else {
		printTable(results)
	}

	return nil
}

// printTable outputs a terse table of results.
func printTable(results []replay.BatchResult) {
	fmt.Printf("%-15s  %6s  %-30s  %8s  %s\n",
		"LABEL", "STATUS", "CONTENT-TYPE", "SIZE", "ERROR")
	fmt.Println(strings.Repeat("-", 80))

	for _, r := range results {
		ct := ""
		size := 0
		if r.Response != nil {
			for _, h := range r.Response.Headers {
				if strings.EqualFold(h.Name, "content-type") {
					ct = strings.SplitN(h.Value, ";", 2)[0]
				}
			}
			size = r.Response.BodySize
		}

		errStr := ""
		if r.Error != "" {
			errStr = r.Error
			if len(errStr) > 40 {
				errStr = errStr[:40] + "..."
			}
		}

		fmt.Printf("%-15s  %6d  %-30s  %6dB  %s\n",
			truncate(r.Label, 15),
			r.StatusCode,
			truncate(ct, 30),
			size,
			errStr,
		)
	}
}

// printVerbose outputs full response details.
func printVerbose(results []replay.BatchResult) {
	for i, r := range results {
		if i > 0 {
			fmt.Println("===")
		}
		fmt.Printf("[%s] ", r.Label)
		if r.Error != "" {
			fmt.Printf("ERROR: %s\n", r.Error)
			continue
		}
		if r.Response != nil {
			fmt.Printf("%d %dms\n", r.StatusCode, r.RoundtripMs)
			fmt.Println(fmtResp(r.Response, false))
		} else {
			fmt.Println("(no response)")
		}
	}
}

// --- Helpers ---

func getHeaders(cmd *cobra.Command) map[string]string {
	raw, _ := cmd.Flags().GetStringArray("header")
	hdrs := make(map[string]string)
	for _, h := range raw {
		if idx := strings.Index(h, ":"); idx > 0 {
			hdrs[strings.TrimSpace(h[:idx])] = strings.TrimSpace(h[idx+1:])
		}
	}
	return hdrs
}

func getBody(cmd *cobra.Command, hdrs map[string]string) string {
	j, _ := cmd.Flags().GetString("json")
	if j != "" {
		if _, ok := hdrs["Content-Type"]; !ok {
			hdrs["Content-Type"] = "application/json"
		}
		return resolveBody(j)
	}
	d, _ := cmd.Flags().GetString("data")
	if d != "" {
		return resolveBody(d)
	}
	return ""
}

func copyHeaders(src map[string]string) map[string]string {
	dst := make(map[string]string, len(src))
	maps.Copy(dst, src)
	return dst
}

func applyToken(hdrs map[string]string, token, authMode string) {
	if token == "" {
		return
	}
	switch {
	case strings.HasPrefix(authMode, "cookie:"):
		name := authMode[7:]
		cookie := name + "=" + token
		if existing, ok := hdrs["Cookie"]; ok && existing != "" {
			hdrs["Cookie"] = existing + "; " + cookie
		} else {
			hdrs["Cookie"] = cookie
		}
	case strings.HasPrefix(authMode, "header:"):
		name := authMode[7:]
		hdrs[name] = token
	default:
		if strings.HasPrefix(token, "Bearer ") ||
			strings.HasPrefix(token, "Basic ") {
			hdrs["Authorization"] = token
		} else {
			hdrs["Authorization"] = "Bearer " + token
		}
	}
}

func applyTokenMap(
	hdrs map[string]string, token, authMode string,
) {
	applyToken(hdrs, token, authMode)
}

func parseTokens(raw string) []tokenDef {
	if raw == "" {
		return nil
	}
	var tokens []tokenDef
	autoIdx := 1

	for p := range strings.SplitSeq(raw, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if p == "noauth" || p == "none" {
			tokens = append(tokens, tokenDef{
				Label: "noauth", Value: "",
			})
			continue
		}
		if idx := strings.Index(p, "="); idx > 0 {
			label := p[:idx]
			if !strings.HasPrefix(label, "eyJ") && len(label) < 30 {
				tokens = append(tokens, tokenDef{
					Label: label, Value: p[idx+1:],
				})
				continue
			}
		}
		tokens = append(tokens, tokenDef{
			Label: fmt.Sprintf("tok-%d", autoIdx),
			Value: p,
		})
		autoIdx++
	}
	return tokens
}

func splitCSV(raw string) []string {
	var out []string
	for v := range strings.SplitSeq(raw, ",") {
		v = strings.TrimSpace(v)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func replaceParam(baseURL, param, value string) string {
	if strings.Contains(baseURL, param+"=") {
		parts := strings.SplitN(baseURL, "?", 2)
		if len(parts) == 2 {
			params := strings.Split(parts[1], "&")
			for i, p := range params {
				if strings.HasPrefix(p, param+"=") {
					params[i] = param + "=" + value
				}
			}
			return parts[0] + "?" + strings.Join(params, "&")
		}
	}
	sep := "?"
	if strings.Contains(baseURL, "?") {
		sep = "&"
	}
	return baseURL + sep + param + "=" + value
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

// buildRawRequest builds a raw HTTP request from structured input.
// Uses the same logic as send.go's buildRequest but returns the
// raw string instead of split components, plus connection info.
func buildRawRequest(
	method, rawURL string,
	hdrs map[string]string,
	body string,
) (raw string, host string, port int, tls bool, err error) {
	// Convert map headers to slice for buildRequest.
	var hdrSlice []string
	for k, v := range hdrs {
		hdrSlice = append(hdrSlice, k+": "+v)
	}
	rawHTTP, h, p, t, buildErr := buildRequest(
		method, rawURL, hdrSlice, body,
	)
	return rawHTTP, h, p, t, buildErr
}

// buildRawRequestFromMap is the same but takes map headers directly.
func buildRawRequestFromMap(
	method, rawURL string,
	hdrs map[string]string,
	body string,
) (raw string, host string, port int, tls bool, err error) {
	return buildRawRequest(method, rawURL, hdrs, body)
}

// batchFileSpec mirrors burp-batch's JSON file format.
type batchFileSpec struct {
	Base struct {
		URL      string            `json:"url"`
		Method   string            `json:"method"`
		Headers  map[string]string `json:"headers"`
		Body     string            `json:"body"`
		AuthMode string            `json:"auth_mode"`
	} `json:"base"`
	Requests []struct {
		Label    string            `json:"label"`
		URL      string            `json:"url"`
		Method   string            `json:"method"`
		Headers  map[string]string `json:"headers"`
		Body     string            `json:"body"`
		Token    string            `json:"token"`
		AuthMode string            `json:"auth_mode"`
	} `json:"requests"`
}
