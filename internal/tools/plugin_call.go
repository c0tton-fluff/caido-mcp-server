package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/c0tton-fluff/caido-mcp-server/v4/internal/auth"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	// pluginCallDefaultMaxBytes is how much of a function's return text is
	// handed to the model by default.
	pluginCallDefaultMaxBytes = 20000
	// pluginCallMaxBytes is the most an agent can ask for. 403Bypasser's
	// getScans/getTemplateResults return every template result, a couple of
	// megabytes on a few thousand templates.
	pluginCallMaxBytes = 8_000_000
	// pluginCallTimeout bounds one plugin function call. Most functions
	// return immediately; 403Bypasser's runScan runs synchronously and its
	// own timeout is five minutes, so this matches it.
	pluginCallTimeout = 5 * time.Minute
	// pluginCallResponseLimit rejects absurd responses before they are
	// decoded into memory.
	pluginCallResponseLimit = 16 << 20
)

// PluginCallInput is the input for the call_plugin tool.
type PluginCallInput struct {
	Plugin string `json:"plugin" jsonschema:"required,Installed plugin: manifestId (quickssrf), name (QuickSSRF), package id or backend id"`
	// Function is the name passed to sdk.api.register in the plugin backend.
	Function string `json:"function" jsonschema:"required,Function registered by the plugin backend (sdk.api.register)"`
	// Args are JSON values. Each is JSON-encoded into its own string, which
	// is what the plugin RPC route expects.
	Args []any `json:"args,omitempty" jsonschema:"Arguments, one JSON value each; each is JSON-encoded into its own string"`
	// MaxBytes caps the returned text, not the parsed result size.
	MaxBytes int `json:"maxBytes,omitempty" jsonschema:"Max bytes of the returned text (default 20000, max 8000000)"`
}

// PluginCallOutput is the result of one plugin backend function call.
type PluginCallOutput struct {
	// Plugin is the package display name.
	Plugin string `json:"plugin"`
	// ManifestID is the package manifestId, the stable plugin identifier.
	ManifestID string `json:"manifestId"`
	// BackendID is the PluginBackend id the call was sent to.
	BackendID string `json:"backendId"`
	Function  string `json:"function"`
	// Result is the plugin's returns value parsed as JSON, or the raw string
	// when it is not JSON (or was truncated).
	Result any `json:"result"`
	// Truncated reports that Result is the first MaxBytes of the text.
	Truncated bool `json:"truncated,omitempty"`
	// TotalBytes is the size of the untruncated returns text.
	TotalBytes int `json:"totalBytes"`
}

// pluginFunctionResponse is the REST envelope of the plugin function route.
type pluginFunctionResponse struct {
	Kind    string          `json:"kind"`
	Returns *string         `json:"returns"`
	Error   json.RawMessage `json:"error"`
}

// pluginFunctionError is the error object inside an envelope of kind "error".
type pluginFunctionError struct {
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Message string `json:"message"`
}

// pluginHTTPClient is shared so connections are reused across calls.
var pluginHTTPClient = &http.Client{Timeout: pluginCallTimeout}

// callPluginHandler creates the handler for the call_plugin tool.
func callPluginHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, PluginCallInput) (*mcp.CallToolResult, PluginCallOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input PluginCallInput,
	) (*mcp.CallToolResult, PluginCallOutput, error) {
		function := strings.TrimSpace(input.Function)
		if function == "" {
			return nil, PluginCallOutput{}, fmt.Errorf("function is required")
		}

		packages, err := listPluginPackages(ctx, client)
		if err != nil {
			return nil, PluginCallOutput{}, err
		}
		target, err := resolvePluginTarget(packages, input.Plugin)
		if err != nil {
			return nil, PluginCallOutput{}, err
		}

		args, err := encodePluginArgs(input.Args)
		if err != nil {
			return nil, PluginCallOutput{}, err
		}
		maxBytes := input.MaxBytes
		if maxBytes <= 0 {
			maxBytes = pluginCallDefaultMaxBytes
		}
		maxBytes = min(maxBytes, pluginCallMaxBytes)

		token, err := auth.ResolveAccessToken(ctx, client)
		if err != nil {
			return nil, PluginCallOutput{}, err
		}

		returns, err := callPluginFunction(
			ctx, client.BaseURL(), target.Backend.ID, function, args, token,
		)
		if err != nil {
			return nil, PluginCallOutput{}, err
		}

		output := PluginCallOutput{
			Plugin:     target.Package.displayName(),
			ManifestID: target.Package.ManifestID,
			BackendID:  target.Backend.ID,
			Function:   function,
		}
		if returns != nil {
			text := *returns
			output.TotalBytes = len(text)
			if strings.TrimSpace(text) == "null" {
				text = ""
			}
			if len(text) > maxBytes {
				text = truncatePluginText(text, maxBytes)
				output.Truncated = true
			}
			if text != "" {
				if output.Truncated {
					// A cut JSON document is not valid JSON; hand the agent
					// the text it can still read.
					output.Result = text
				} else if json.Valid([]byte(text)) {
					var parsed any
					if err := json.Unmarshal([]byte(text), &parsed); err == nil {
						output.Result = parsed
					} else {
						output.Result = text
					}
				} else {
					output.Result = text
				}
			}
		}

		return nil, output, nil
	}
}

// encodePluginArgs JSON-encodes each argument into its own string, the wire
// form the plugin RPC route expects.
func encodePluginArgs(args []any) ([]string, error) {
	encoded := make([]string, 0, len(args))
	for i, arg := range args {
		data, err := json.Marshal(arg)
		if err != nil {
			return nil, fmt.Errorf("args[%d] is not valid JSON: %w", i, err)
		}
		encoded = append(encoded, string(data))
	}
	return encoded, nil
}

// callPluginFunction posts one function call to the plugin backend route and
// returns the envelope's returns value. A nil result means the plugin returned
// JSON null.
func callPluginFunction(
	ctx context.Context,
	baseURL, backendID, function string,
	args []string,
	token string,
) (*string, error) {
	payload, err := json.Marshal(struct {
		Name string   `json:"name"`
		Args []string `json:"args"`
	}{Name: function, Args: args})
	if err != nil {
		return nil, fmt.Errorf("encode plugin call: %w", err)
	}

	endpoint := strings.TrimRight(baseURL, "/") + "/plugin/backend/" +
		url.PathEscape(backendID) + "/function"
	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, endpoint, bytes.NewReader(payload),
	)
	if err != nil {
		return nil, fmt.Errorf("build plugin call: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := pluginHTTPClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf(
				"plugin call did not return within %s: the function may "+
					"still be running inside the plugin; read its state "+
					"with another function",
				pluginCallTimeout,
			)
		}
		return nil, fmt.Errorf("plugin call failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(io.LimitReader(resp.Body, pluginCallResponseLimit+1))
	if err != nil {
		return nil, fmt.Errorf("read plugin response: %w", err)
	}
	if len(data) > pluginCallResponseLimit {
		return nil, fmt.Errorf(
			"plugin response exceeded %d bytes; call a function that "+
				"returns less, or page it in the plugin",
			pluginCallResponseLimit,
		)
	}

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized:
		return nil, fmt.Errorf(
			"plugin call rejected: HTTP 401 (INVALID_TOKEN); the access " +
				"token is invalid or expired, run \"caido-mcp-server login\" " +
				"or set CAIDO_ACCESS_TOKEN",
		)
	case http.StatusInternalServerError:
		return nil, pluginBackendUnavailableError(
			fmt.Sprintf("HTTP 500: %s", bodySnippet(data)),
		)
	default:
		return nil, fmt.Errorf(
			"plugin call failed: HTTP %d: %s",
			resp.StatusCode, bodySnippet(data),
		)
	}

	var envelope pluginFunctionResponse
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf(
			"plugin call returned a non-JSON body: %s", bodySnippet(data),
		)
	}

	switch envelope.Kind {
	case "success":
		if envelope.Returns != nil && *envelope.Returns == "null" {
			return nil, nil
		}
		return envelope.Returns, nil
	case "error":
		var callErr pluginFunctionError
		_ = json.Unmarshal(envelope.Error, &callErr)
		if callErr.Kind == "invalid_procedure" {
			return nil, fmt.Errorf(
				"plugin function %q is not registered; function names are "+
					"the sdk.api.register(\"...\") calls in the plugin's "+
					"backend source (see its repository)",
				function,
			)
		}
		return nil, fmt.Errorf(
			"plugin function %q failed: %s", function, bodySnippet(envelope.Error),
		)
	default:
		return nil, fmt.Errorf(
			"plugin call returned an unexpected envelope: %s",
			bodySnippet(data),
		)
	}
}

// pluginBackendUnavailableError explains the Caido 0.59 failure mode where a
// plugin backend installed while Caido is running has no live runtime.
func pluginBackendUnavailableError(detail string) error {
	return fmt.Errorf(
		"plugin backend is not available (%s): restart Caido; plugins "+
			"installed while Caido is running are not callable until then",
		detail,
	)
}

// truncatePluginText cuts s to at most max bytes without splitting a UTF-8
// rune.
func truncatePluginText(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

// bodySnippet trims a response body for an error message.
func bodySnippet(body []byte) string {
	const limit = 300
	text := strings.TrimSpace(string(body))
	if len(text) > limit {
		text = text[:limit] + "..."
	}
	if text == "" {
		return "(empty body)"
	}
	return text
}

// RegisterCallPluginTool registers the call_plugin tool.
func RegisterCallPluginTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:  "caido_call_plugin",
		Title: "Call Plugin Function",
		Description: `Call a function registered by a Caido plugin backend (` +
			`sdk.api.register). plugin matches an installed package manifestId, ` +
			`name, package id or backend id, case-insensitively; function is ` +
			`the registered name; args are JSON values, each sent as its own ` +
			`JSON-encoded string. Function names and argument shapes come from ` +
			`the plugin's backend source (the sdk.api.register("...") calls in ` +
			`its repository); caido_list_plugins shows what is installed and ` +
			`callable. Plugin functions can send traffic, change plugin state ` +
			`or delete data, and scope enforcement cannot see their targets. Example: plugin:"quickssrf", ` +
			`function:"createSession", args:[] returns a Session with a url; ` +
			`request that url, then plugin:"quickssrf", ` +
			`function:"getInteractions", args:["<session id>"] lists the hit.`,
		Annotations: writeAnn(true, false, true),
	}, callPluginHandler(client))
}
