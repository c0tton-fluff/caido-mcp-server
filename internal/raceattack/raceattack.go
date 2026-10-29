// Package raceattack provides a synchronized, last-byte HTTP/1.1 request
// sender for race-condition testing (single-packet / last-byte sync style).
//
// IMPORTANT: This package INTENTIONALLY bypasses the Caido proxy. It dials
// the target directly with raw Go net/tls sockets, so requests sent here do
// NOT appear in Caido history. Caido's draft-then-start replay cannot park
// connections at a shared barrier, so before Caido 0.59 raw sockets from this
// process were the only way to do so. Caido 0.59 Replay Pipelines do it inside
// the proxy (see replay.RunPipeline / caido_race_pipeline); this package stays
// for older servers and raw-socket control.
//
// HONESTY NOTE: The barrier guarantees that every goroutine has written its
// primer and reached the send point before ANY final byte is written. It does
// NOT guarantee wall-clock wire simultaneity -- actual arrival timing is
// best-effort and depends on the OS scheduler and network path. DurationMs is
// informational only.
package raceattack

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// dialTimeout bounds how long a single connection dial may take.
const dialTimeout = 10 * time.Second

// idleReadTimeout bounds how long readResponse waits for more body bytes
// before concluding the peer has stopped sending. A keep-alive connection
// stays open after a complete response, so without an idle bound a short
// body would block until dialTimeout waiting to fill the capture buffer.
const idleReadTimeout = 500 * time.Millisecond

// defaultBodyLimit and maxBodyLimit bound the per-response body capture.
const (
	defaultBodyLimit = 4096
	maxBodyLimit     = 65536
	maxRequests      = 50
)

// Target identifies the host to connect to for every request.
type Target struct {
	Host string
	Port int
	TLS  bool
}

// Request is a single raw HTTP/1.1 request to send across the barrier.
type Request struct {
	Label string
	Raw   string // raw HTTP/1.1 bytes
}

// Result captures the outcome of a single synchronized request.
type Result struct {
	Label      string `json:"label"`
	StatusLine string `json:"statusLine"`
	StatusCode int    `json:"statusCode"`
	Body       string `json:"body"`
	Error      string `json:"error,omitempty"`
	DurationMs int64  `json:"durationMs"`
}

// conn pairs a dialed connection with the request that owns it. A nil net.Conn
// means the dial failed and res.Error is already populated.
type conn struct {
	net.Conn
	req Request
	res *Result
}

// Send dials every request connection in parallel, writes all but the last
// byte of each, parks each goroutine at a shared barrier, then releases them
// together to write their final byte and read the response. Dial failures are
// recorded in the corresponding Result and do not abort the rest. Cancelling
// ctx aborts dials and closes live connections, so reads return promptly. All
// connections are closed before Send returns.
func Send(ctx context.Context, target Target, requests []Request, bodyLimit int) []Result {
	if err := validate(target, requests); err != nil {
		return []Result{{Label: "validation", Error: err.Error()}}
	}
	bodyLimit = clampBodyLimit(bodyLimit)

	results := make([]Result, len(requests))
	conns := make([]conn, len(requests))
	for i, r := range requests {
		results[i] = Result{Label: r.Label}
		conns[i] = conn{req: r, res: &results[i]}
	}
	defer closeAll(conns)

	release := make(chan struct{})
	var primed, done sync.WaitGroup
	for i := range conns {
		primed.Add(1)
		done.Add(1)
		go runOne(ctx, target, &conns[i], release, &primed, &done, bodyLimit)
	}

	primed.Wait() // every goroutine is parked at the barrier or has failed
	close(release)
	done.Wait()
	return results
}

// runOne dials, writes the primer, signals readiness, blocks on the barrier,
// then writes the final byte and reads the response. primed.Done runs exactly
// once on every path (including dial and primer failures) so a failed
// connection can never hold the barrier shut for the others.
func runOne(
	ctx context.Context, target Target, c *conn,
	release <-chan struct{}, primed, done *sync.WaitGroup, bodyLimit int,
) {
	defer done.Done()
	nc, err := dialOne(ctx, target)
	if err != nil {
		c.res.Error = fmt.Sprintf("dial: %v", err)
		primed.Done()
		return
	}
	c.Conn = nc
	// Closing the conn on cancel unblocks any pending read or write; a read
	// deadline would be overwritten by readBody's per-read idle deadline.
	stop := context.AfterFunc(ctx, func() { _ = nc.Close() })
	defer stop()

	last, err := writePrimer(c.Conn, c.req.Raw)
	primed.Done()
	if err != nil {
		c.res.Error = fmt.Sprintf("primer write: %v", err)
		<-release
		return
	}
	<-release
	start := time.Now()
	if _, werr := c.Write(last); werr != nil {
		c.res.Error = fmt.Sprintf("final write: %v", werr)
		return
	}
	readResponse(c.Conn, c.res, bodyLimit)
	c.res.DurationMs = time.Since(start).Milliseconds()
}

// dialOne opens a single TCP (or TLS) connection to the target.
func dialOne(ctx context.Context, target Target) (net.Conn, error) {
	addr := net.JoinHostPort(target.Host, strconv.Itoa(target.Port))
	dialer := &net.Dialer{Timeout: dialTimeout}
	// Bound the TLS handshake too: net.Dialer.Timeout only covers the TCP
	// connect. The conn does not depend on ctx after the dial returns.
	ctx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()
	if !target.TLS {
		return dialer.DialContext(ctx, "tcp", addr)
	}
	cfg := &tls.Config{ServerName: target.Host, InsecureSkipVerify: true} //nolint:gosec // race testing tool dials arbitrary targets
	return (&tls.Dialer{NetDialer: dialer, Config: cfg}).DialContext(ctx, "tcp", addr)
}

// writePrimer writes everything except the final byte and flushes, returning
// the withheld last byte. An empty raw yields an empty final byte slice.
func writePrimer(c net.Conn, raw string) ([]byte, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	primer := raw[:len(raw)-1]
	last := []byte{raw[len(raw)-1]}
	if _, err := io.WriteString(c, primer); err != nil {
		return last, err
	}
	return last, nil
}

// readResponse reads the status line, headers, and up to bodyLimit body bytes.
func readResponse(c net.Conn, res *Result, bodyLimit int) {
	_ = c.SetReadDeadline(time.Now().Add(dialTimeout))
	reader := bufio.NewReader(c)
	statusLine, err := reader.ReadString('\n')
	if err != nil && statusLine == "" {
		res.Error = fmt.Sprintf("read status: %v", err)
		return
	}
	res.StatusLine = strings.TrimRight(statusLine, "\r\n")
	res.StatusCode = parseStatus(res.StatusLine)
	for {
		line, herr := reader.ReadString('\n')
		if strings.TrimRight(line, "\r\n") == "" || herr != nil {
			break
		}
	}
	res.Body = readBody(c, reader, bodyLimit)
}

// readBody reads up to bodyLimit body bytes, returning promptly once the
// peer stops sending. It applies a short idle deadline per read so a
// keep-alive connection with a complete small body does not block for the
// full dialTimeout waiting to fill a fixed buffer, while a body larger than
// bodyLimit is still truncated to bodyLimit.
func readBody(c net.Conn, reader *bufio.Reader, bodyLimit int) string {
	body := make([]byte, 0, bodyLimit)
	buf := make([]byte, 4096)
	for len(body) < bodyLimit {
		_ = c.SetReadDeadline(time.Now().Add(idleReadTimeout))
		want := len(buf)
		if remaining := bodyLimit - len(body); remaining < want {
			want = remaining
		}
		n, err := reader.Read(buf[:want])
		if n > 0 {
			body = append(body, buf[:n]...)
		}
		if err != nil {
			break
		}
	}
	return string(body)
}

// parseStatus extracts the numeric status code from an HTTP/1.1 status line.
// Returns 0 when the status line cannot be parsed.
func parseStatus(statusLine string) int {
	parts := strings.SplitN(statusLine, " ", 3)
	if len(parts) < 2 {
		return 0
	}
	code, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0
	}
	return code
}

// validate enforces target and request-count constraints.
func validate(target Target, requests []Request) error {
	if strings.TrimSpace(target.Host) == "" {
		return fmt.Errorf("target host must not be empty")
	}
	if target.Port < 1 || target.Port > 65535 {
		return fmt.Errorf("target port must be 1..65535, got %d", target.Port)
	}
	if len(requests) == 0 {
		return fmt.Errorf("at least one request is required")
	}
	if len(requests) > maxRequests {
		return fmt.Errorf("max %d requests, got %d", maxRequests, len(requests))
	}
	return nil
}

// clampBodyLimit applies the default and maximum body capture bounds.
func clampBodyLimit(bodyLimit int) int {
	if bodyLimit <= 0 {
		return defaultBodyLimit
	}
	if bodyLimit > maxBodyLimit {
		return maxBodyLimit
	}
	return bodyLimit
}

// closeAll closes every open connection, ignoring close errors.
func closeAll(conns []conn) {
	for i := range conns {
		if conns[i].Conn != nil {
			_ = conns[i].Close()
		}
	}
}
