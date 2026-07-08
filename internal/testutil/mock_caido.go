package testutil

import (
	"encoding/json"
	"io"
	"net/http"
	"sync"
)

type MockHandler struct {
	mu            sync.Mutex
	responses     map[string][]json.RawMessage
	lastVariables map[string]map[string]any
}

func NewMockHandler() *MockHandler {
	return &MockHandler{
		responses:     make(map[string][]json.RawMessage),
		lastVariables: make(map[string]map[string]any),
	}
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
		Variables     map[string]any `json:"variables"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	m.mu.Lock()
	m.lastVariables[req.OperationName] = req.Variables
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
		json.NewEncoder(w).Encode(map[string]any{
			"errors": []map[string]string{
				{"message": "no mock for operation: " + req.OperationName},
			},
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]json.RawMessage{
		"data": resp,
	})
}

// LastVariables returns the most recently decoded GraphQL "variables" object
// sent for the given operation name, or nil if that operation has not been
// invoked. Use it to assert the request payload a handler transmitted.
func (m *MockHandler) LastVariables(op string) map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastVariables[op]
}
