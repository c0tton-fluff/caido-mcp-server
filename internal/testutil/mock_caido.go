package testutil

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
)

type MockHandler struct {
	mu              sync.Mutex
	responses       map[string][]json.RawMessage
	lastVariables   map[string]map[string]any
	hang            map[string]bool
	validate        bool
	pluginResponses []mockPluginResponse
	pluginCalls     []PluginCallRecord
}

// PluginCallRecord is one POST to /plugin/backend/{id}/function seen by the
// mock, decoded from the plugin RPC request body.
type PluginCallRecord struct {
	BackendID     string
	Name          string
	Args          []string
	Authorization string
}

// mockPluginResponse is a queued response for the plugin RPC route.
type mockPluginResponse struct {
	status int
	body   string
}

func NewMockHandler() *MockHandler {
	return &MockHandler{
		responses:     make(map[string][]json.RawMessage),
		lastVariables: make(map[string]map[string]any),
		hang:          make(map[string]bool),
	}
}

// Hang makes every call of operationName block until the client gives up
// (its context ends), simulating a Caido mutation that is never answered.
func (m *MockHandler) Hang(operationName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hang[operationName] = true
}

// OnPluginCall queues a response for the plugin function route
// (POST /plugin/backend/{id}/function). Like On, the last response is kept
// for repeated calls.
func (m *MockHandler) OnPluginCall(status int, body string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pluginResponses = append(m.pluginResponses, mockPluginResponse{
		status: status,
		body:   body,
	})
}

// LastPluginCall returns the most recent plugin RPC request the mock served,
// or nil if none arrived.
func (m *MockHandler) LastPluginCall() *PluginCallRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.pluginCalls) == 0 {
		return nil
	}
	call := m.pluginCalls[len(m.pluginCalls)-1]
	return &call
}

// ValidateAgainstSchema makes the mock check every incoming query and its
// variables against the vendored Caido GraphQL schema, returning a GraphQL
// error response when they do not conform. Opt in from any test whose
// subject builds GraphQL input by hand -- without it the mock accepts
// input the real server rejects. See schema_validate.go.
func (m *MockHandler) ValidateAgainstSchema() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.validate = true
}

func (m *MockHandler) On(operationName string, response any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, _ := json.Marshal(response)
	m.responses[operationName] = append(m.responses[operationName], json.RawMessage(data))
}

func (m *MockHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}

	var req struct {
		OperationName string         `json:"operationName"`
		Query         string         `json:"query"`
		Variables     map[string]any `json:"variables"`
	}
	if r.Method == http.MethodPost &&
		strings.HasPrefix(r.URL.Path, "/plugin/backend/") {
		m.servePluginCall(w, r, body)
		return
	}

	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	m.mu.Lock()
	validate := m.validate
	m.mu.Unlock()

	if validate {
		if err := validateOperation(req.Query, req.Variables); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"errors": []map[string]string{
					{"message": "schema validation: " + err.Error()},
				},
			})
			return
		}
	}

	m.mu.Lock()
	m.lastVariables[req.OperationName] = req.Variables
	hang := m.hang[req.OperationName]
	m.mu.Unlock()
	if hang {
		<-r.Context().Done()
		return
	}

	m.mu.Lock()
	queue, ok := m.responses[req.OperationName]
	var resp json.RawMessage
	if ok && len(queue) > 0 {
		resp = queue[0]
		if len(queue) > 1 {
			m.responses[req.OperationName] = queue[1:]
		}
		// When only one item left, keep it for repeated calls
	}
	m.mu.Unlock()

	if !ok || resp == nil {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"errors": []map[string]string{
				{"message": "no mock for operation: " + req.OperationName},
			},
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]json.RawMessage{
		"data": resp,
	})
}

// servePluginCall records and answers one plugin backend function call.
func (m *MockHandler) servePluginCall(
	w http.ResponseWriter, r *http.Request, body []byte,
) {
	var call struct {
		Name string   `json:"name"`
		Args []string `json:"args"`
	}
	_ = json.Unmarshal(body, &call)

	rec := PluginCallRecord{
		BackendID: strings.TrimSuffix(
			strings.TrimPrefix(r.URL.Path, "/plugin/backend/"),
			"/function",
		),
		Name:          call.Name,
		Args:          call.Args,
		Authorization: r.Header.Get("Authorization"),
	}

	m.mu.Lock()
	m.pluginCalls = append(m.pluginCalls, rec)
	resp := mockPluginResponse{status: http.StatusInternalServerError}
	if len(m.pluginResponses) > 0 {
		resp = m.pluginResponses[0]
		if len(m.pluginResponses) > 1 {
			m.pluginResponses = m.pluginResponses[1:]
		}
	}
	m.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.status)
	_, _ = w.Write([]byte(resp.body))
}

// LastVariables returns the most recently decoded GraphQL "variables" object
// sent for the given operation name, or nil if that operation has not been
// invoked. Use it to assert the request payload a handler transmitted.
func (m *MockHandler) LastVariables(op string) map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastVariables[op]
}
