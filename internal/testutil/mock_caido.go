package testutil

import (
	"encoding/json"
	"io"
	"net/http"
	"sync"
)

type MockHandler struct {
	mu        sync.Mutex
	responses map[string]json.RawMessage
}

func NewMockHandler() *MockHandler {
	return &MockHandler{
		responses: make(map[string]json.RawMessage),
	}
}

func (m *MockHandler) On(operationName string, response any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, _ := json.Marshal(response)
	m.responses[operationName] = json.RawMessage(data)
}

func (m *MockHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}

	var req struct {
		OperationName string `json:"operationName"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	m.mu.Lock()
	resp, ok := m.responses[req.OperationName]
	m.mu.Unlock()

	if !ok {
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
