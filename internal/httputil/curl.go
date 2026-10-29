package httputil

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"path"
	"strconv"
	"strings"
	"unicode/utf8"
)

// CurlRequest is a cURL command translated into a raw HTTP/1.1 request and
// its connection target.
type CurlRequest struct {
	Raw  string // CRLF line endings
	Host string
	Port int
	TLS  bool
}

// ParseCurl translates a cURL command line in POSIX shell form (what a
// browser's "Copy as cURL (bash)" produces; the Windows cmd form is not
// supported) into a raw request. It is strict: an option it does not understand is an
// error rather than silently dropped, because a quietly different request
// is worse than none. Options that read files (-d @file, -F, -T) are
// rejected for the same reason. curl's own default User-Agent and Accept
// headers are not added; the request carries exactly what the command
// specifies plus Host (and Content-Type/Content-Length for -d bodies).
func ParseCurl(cmd string) (*CurlRequest, error) {
	args, err := shellSplit(cmd)
	if err != nil {
		return nil, err
	}
	if len(args) == 0 || !isCurlBinary(args[0]) {
		return nil, fmt.Errorf("not a curl command (must start with curl)")
	}

	var (
		rawURL, method      string
		headers             []Header
		removed             = map[string]bool{}
		data                []string
		hasData, get, head  bool
		cookie, user, agent string
		referer             string
	)
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			if rawURL != "" {
				return nil, fmt.Errorf("more than one URL (%q and %q)", rawURL, arg)
			}
			rawURL = arg
			continue
		}
		name, value, hasValue := splitCurlOption(arg)
		opt, ok := curlOptions[name]
		if !ok {
			// A cluster of short flags such as -sSL or -sXPOST.
			if expanded, cerr := expandShortCluster(arg); cerr == nil {
				args = append(args[:i], append(expanded, args[i+1:]...)...)
				i--
				continue
			}
			return nil, fmt.Errorf("unsupported curl option %q", arg)
		}
		if opt.takesValue && !hasValue {
			if i+1 >= len(args) {
				return nil, fmt.Errorf("curl option %s needs a value", name)
			}
			i++
			value = args[i]
		}
		switch opt.kind {
		case optURL:
			if rawURL != "" {
				return nil, fmt.Errorf("more than one URL (%q and %q)", rawURL, value)
			}
			rawURL = value
		case optMethod:
			method = value
		case optHeader:
			h, isRemoval, herr := parseCurlHeader(value)
			if herr != nil {
				return nil, herr
			}
			if isRemoval {
				removed[strings.ToLower(h.Name)] = true
				continue
			}
			headers = append(headers, h)
		case optData, optDataBinary:
			if strings.HasPrefix(value, "@") {
				return nil, fmt.Errorf("%s @file is not supported; paste the body inline", name)
			}
			hasData = true
			data = append(data, value)
		case optDataRaw:
			hasData = true
			data = append(data, value)
		case optDataURLEncode:
			enc, derr := curlURLEncode(value)
			if derr != nil {
				return nil, derr
			}
			hasData = true
			data = append(data, enc)
		case optGet:
			get = true
		case optHead:
			head = true
		case optCookie:
			if !strings.Contains(value, "=") {
				return nil, fmt.Errorf("-b %q reads a cookie file; pass cookies as name=value", value)
			}
			cookie = value
		case optUser:
			user = value
		case optAgent:
			agent = value
		case optReferer:
			referer = value
		case optReject:
			return nil, fmt.Errorf("curl option %s is not supported (%s)", name, opt.why)
		case optIgnore:
		}
	}
	if rawURL == "" {
		return nil, fmt.Errorf("curl command has no URL")
	}

	u, err := parseCurlURL(rawURL)
	if err != nil {
		return nil, err
	}
	body := strings.Join(data, "&")
	if get && hasData {
		if u.RawQuery != "" {
			u.RawQuery += "&" + body
		} else {
			u.RawQuery = body
		}
		hasData, body = false, ""
	}

	switch {
	case method != "":
	case head:
		method = "HEAD"
	case hasData:
		method = "POST"
	default:
		method = "GET"
	}

	useTLS := u.Scheme == "https"
	port := DefaultPort(useTLS)
	if p := u.Port(); p != "" {
		port, err = strconv.Atoi(p)
		if err != nil || port < 1 || port > 65535 {
			return nil, fmt.Errorf("invalid port %q", p)
		}
	}

	target := u.EscapedPath()
	if target == "" {
		target = "/"
	}
	if u.RawQuery != "" {
		target += "?" + u.RawQuery
	}

	has := func(name string) bool {
		for _, h := range headers {
			if strings.EqualFold(h.Name, name) {
				return true
			}
		}
		return false
	}
	add := func(name, value string) {
		if !has(name) && !removed[strings.ToLower(name)] {
			headers = append(headers, Header{Name: name, Value: value})
		}
	}
	if cookie != "" {
		add("Cookie", cookie)
	}
	if user != "" {
		add("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(user)))
	}
	if agent != "" {
		add("User-Agent", agent)
	}
	if referer != "" {
		add("Referer", referer)
	}
	if hasData {
		add("Content-Type", "application/x-www-form-urlencoded")
		add("Content-Length", strconv.Itoa(len(body)))
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s %s HTTP/1.1\r\n", method, target)
	if !has("Host") && !removed["host"] {
		fmt.Fprintf(&b, "Host: %s\r\n", u.Host)
	}
	for _, h := range headers {
		fmt.Fprintf(&b, "%s: %s\r\n", h.Name, h.Value)
	}
	b.WriteString("\r\n")
	b.WriteString(body)

	return &CurlRequest{Raw: b.String(), Host: u.Hostname(), Port: port, TLS: useTLS}, nil
}

type curlOptKind int

const (
	optIgnore curlOptKind = iota
	optReject
	optURL
	optMethod
	optHeader
	optData
	optDataRaw
	optDataBinary
	optDataURLEncode
	optGet
	optHead
	optCookie
	optUser
	optAgent
	optReferer
)

type curlOpt struct {
	kind       curlOptKind
	takesValue bool
	why        string // for optReject
}

// curlOptions maps every option ParseCurl understands. Transport-only
// options that do not change the request bytes are accepted and ignored.
var curlOptions = map[string]curlOpt{
	"--url": {kind: optURL, takesValue: true},
	"-X":    {kind: optMethod, takesValue: true}, "--request": {kind: optMethod, takesValue: true},
	"-H": {kind: optHeader, takesValue: true}, "--header": {kind: optHeader, takesValue: true},
	"-d": {kind: optData, takesValue: true}, "--data": {kind: optData, takesValue: true},
	"--data-ascii":     {kind: optData, takesValue: true},
	"--data-binary":    {kind: optDataBinary, takesValue: true},
	"--data-raw":       {kind: optDataRaw, takesValue: true},
	"--data-urlencode": {kind: optDataURLEncode, takesValue: true},
	"-G":               {kind: optGet}, "--get": {kind: optGet},
	"-I": {kind: optHead}, "--head": {kind: optHead},
	"-b": {kind: optCookie, takesValue: true}, "--cookie": {kind: optCookie, takesValue: true},
	"-u": {kind: optUser, takesValue: true}, "--user": {kind: optUser, takesValue: true},
	"-A": {kind: optAgent, takesValue: true}, "--user-agent": {kind: optAgent, takesValue: true},
	"-e": {kind: optReferer, takesValue: true}, "--referer": {kind: optReferer, takesValue: true},

	"-F":            {kind: optReject, takesValue: true, why: "multipart form; build the body in raw instead"},
	"--form":        {kind: optReject, takesValue: true, why: "multipart form; build the body in raw instead"},
	"-T":            {kind: optReject, takesValue: true, why: "reads a file"},
	"--upload-file": {kind: optReject, takesValue: true, why: "reads a file"},
	"--json":        {kind: optReject, takesValue: true, why: "use -H 'Content-Type: application/json' --data-raw"},

	"-k": {}, "--insecure": {}, "-L": {}, "--location": {}, "-s": {}, "--silent": {},
	"-S": {}, "--show-error": {}, "-i": {}, "--include": {}, "-v": {}, "--verbose": {},
	"--compressed": {}, "--http1.0": {}, "--http1.1": {}, "--http2": {}, "-f": {}, "--fail": {},
	"-N": {}, "--no-buffer": {}, "--path-as-is": {}, "-g": {}, "--globoff": {},
	"-o": {takesValue: true}, "--output": {takesValue: true},
	"-w": {takesValue: true}, "--write-out": {takesValue: true},
	"-m": {takesValue: true}, "--max-time": {takesValue: true},
	"--connect-timeout": {takesValue: true}, "--retry": {takesValue: true},
	"-x": {takesValue: true}, "--proxy": {takesValue: true},
	"-c": {takesValue: true}, "--cookie-jar": {takesValue: true},
}

// splitCurlOption separates an attached short-option value (-XPOST) from
// its option. Long options never take attached values in curl.
func splitCurlOption(arg string) (name, value string, hasValue bool) {
	if strings.HasPrefix(arg, "--") || len(arg) <= 2 {
		return arg, "", false
	}
	name = arg[:2]
	if opt, ok := curlOptions[name]; ok && opt.takesValue {
		return name, arg[2:], true
	}
	return arg, "", false
}

// expandShortCluster splits -sSL into -s -S -L, and -sXPOST into -s -XPOST.
func expandShortCluster(arg string) ([]string, error) {
	if strings.HasPrefix(arg, "--") || len(arg) < 3 {
		return nil, fmt.Errorf("not a cluster")
	}
	var out []string
	for j := 1; j < len(arg); j++ {
		flag := "-" + string(arg[j])
		opt, ok := curlOptions[flag]
		if !ok {
			return nil, fmt.Errorf("unknown flag %s", flag)
		}
		if opt.takesValue {
			out = append(out, flag)
			if rest := arg[j+1:]; rest != "" {
				out = append(out, rest)
			}
			return out, nil
		}
		out = append(out, flag)
	}
	return out, nil
}

func isCurlBinary(s string) bool {
	base := strings.ToLower(path.Base(strings.ReplaceAll(s, `\`, "/")))
	return base == "curl" || base == "curl.exe"
}

// parseCurlHeader parses -H "Name: value". "Name:" (nothing after the
// colon) removes a header in curl; "Name;" sends it empty.
func parseCurlHeader(v string) (Header, bool, error) {
	if name, ok := strings.CutSuffix(v, ";"); ok && !strings.Contains(name, ":") {
		return Header{Name: strings.TrimSpace(name)}, false, nil
	}
	name, value, ok := strings.Cut(v, ":")
	name = strings.TrimSpace(name)
	if !ok || name == "" {
		return Header{}, false, fmt.Errorf("invalid header %q (want Name: value)", v)
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return Header{Name: name}, true, nil
	}
	return Header{Name: name, Value: value}, false, nil
}

// curlURLEncode implements --data-urlencode's content forms.
func curlURLEncode(v string) (string, error) {
	name, content, ok := strings.Cut(v, "=")
	if !ok && strings.Contains(v, "@") {
		// "@file" and "name@file" read a file in curl.
		return "", fmt.Errorf("--data-urlencode with @file is not supported")
	}
	if ok {
		if name == "" {
			return url.QueryEscape(content), nil
		}
		return name + "=" + url.QueryEscape(content), nil
	}
	return url.QueryEscape(v), nil
}

// parseCurlURL parses the target URL, defaulting the scheme to http as
// curl does.
func parseCurlURL(raw string) (*url.URL, error) {
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid URL %q: %w", raw, err)
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("unsupported URL scheme %q (http or https only)", u.Scheme)
	}
	if u.Hostname() == "" {
		return nil, fmt.Errorf("URL %q has no host", raw)
	}
	return u, nil
}

// shellSplit tokenizes a POSIX shell command line: single quotes, double
// quotes, $'...' ANSI-C quotes, backslash escapes and line continuations.
func shellSplit(s string) ([]string, error) {
	var (
		args  []string
		cur   strings.Builder
		inTok bool
	)
	flush := func() {
		if inTok {
			args = append(args, cur.String())
			cur.Reset()
			inTok = false
		}
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\' && i+1 < len(s) && (s[i+1] == '\n' || s[i+1] == '\r'):
			// Line continuation.
			i++
			if s[i] == '\r' && i+1 < len(s) && s[i+1] == '\n' {
				i++
			}
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			flush()
		case c == '\\':
			inTok = true
			if i+1 < len(s) {
				i++
				cur.WriteByte(s[i])
			}
		case c == '\'':
			inTok = true
			end := strings.IndexByte(s[i+1:], '\'')
			if end < 0 {
				return nil, fmt.Errorf("unterminated single quote")
			}
			cur.WriteString(s[i+1 : i+1+end])
			i += end + 1
		case c == '$' && i+1 < len(s) && s[i+1] == '\'':
			inTok = true
			n, err := ansiCQuote(s[i+2:], &cur)
			if err != nil {
				return nil, err
			}
			i += 1 + n
		case c == '"':
			inTok = true
			j := i + 1
			for ; j < len(s) && s[j] != '"'; j++ {
				if s[j] == '\\' && j+1 < len(s) && strings.IndexByte("\"\\$`\n", s[j+1]) >= 0 {
					j++
					if s[j] != '\n' {
						cur.WriteByte(s[j])
					}
					continue
				}
				cur.WriteByte(s[j])
			}
			if j >= len(s) {
				return nil, fmt.Errorf("unterminated double quote")
			}
			i = j
		default:
			inTok = true
			cur.WriteByte(c)
		}
	}
	flush()
	return args, nil
}

// ansiCQuote decodes the body of a $'...' string (s starts after the
// opening quote) into cur and returns the bytes consumed including the
// closing quote.
func ansiCQuote(s string, cur *strings.Builder) (int, error) {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\'' {
			return i + 1, nil
		}
		if c != '\\' || i+1 >= len(s) {
			cur.WriteByte(c)
			continue
		}
		i++
		switch e := s[i]; e {
		case 'n':
			cur.WriteByte('\n')
		case 'r':
			cur.WriteByte('\r')
		case 't':
			cur.WriteByte('\t')
		case '0':
			cur.WriteByte(0)
		case '\\', '\'', '"', '?':
			cur.WriteByte(e)
		case 'x', 'u', 'U':
			width := map[byte]int{'x': 2, 'u': 4, 'U': 8}[e]
			j := i + 1
			for j < len(s) && j-i-1 < width && isHex(s[j]) {
				j++
			}
			if j == i+1 {
				return 0, fmt.Errorf(`invalid \%c escape in $'...'`, e)
			}
			v, _ := strconv.ParseUint(s[i+1:j], 16, 32)
			if e == 'x' {
				cur.WriteByte(byte(v))
			} else {
				var buf [utf8.UTFMax]byte
				cur.Write(buf[:utf8.EncodeRune(buf[:], rune(v))])
			}
			i = j - 1
		default:
			cur.WriteByte('\\')
			cur.WriteByte(e)
		}
	}
	return 0, fmt.Errorf("unterminated $'...' quote")
}

func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}
