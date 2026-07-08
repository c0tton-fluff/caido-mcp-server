// Package raceattack provides a synchronized, last-byte HTTP/1.1 request
// sender for race-condition testing (single-packet / last-byte sync style).
//
// IMPORTANT: This package INTENTIONALLY bypasses the Caido proxy. It dials
// the target directly with raw Go net/tls sockets, so requests sent here do
// NOT appear in Caido history. Caido's SDK/GraphQL replay polls at 50ms and
// cannot produce a sub-millisecond race window; raw sockets from this process
// are the only way to park multiple connections at a shared barrier before
// the final bytes are written.
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

// Send dials every request connection, writes all but the last byte of each,
// parks each goroutine at a shared barrier, then releases them together to
// write their final byte and read the response. Dial failures are recorded in
// the corresponding Result and do not abort the rest. All connections are
// closed before Send returns.
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
		c, err := dialOne(ctx, target)
		if err != nil {
			results[i].Error = fmt.Sprintf("dial: %v", err)
			continue
		}
		conns[i].Conn = c
	}
	defer closeAll(conns)

	release := make(chan struct{})
	var primed, done sync.WaitGroup
	for i := range conns {
		if conns[i].Conn == nil {
			continue
		}
		primed.Add(1)
		done.Add(1)
		go runOne(&conns[i], release, &primed, &done, bodyLimit)
	}

	primed.Wait() // all goroutines parked at the barrier
	close(release)
	done.Wait()
	return results
}

// runOne writes the primer, signals readiness, blocks on the barrier, then
// writes the final byte and reads the response.
func runOne(c *conn, release <-chan struct{}, primed, done *sync.WaitGroup, bodyLimit int) {
	defer done.Done()
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
	if !target.TLS {
		return dialer.DialContext(ctx, "tcp", addr)
	}
	cfg := &tls.Config{ServerName: target.Host, InsecureSkipVerify: true} //nolint:gosec // race testing tool dials arbitrary targets
	return tls.DialWithDialer(dialer, "tcp", addr, cfg)
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
