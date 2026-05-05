package testutil

import (
	"net/http/httptest"
	"testing"

	caido "github.com/caido-community/sdk-go"
)

type TestEnv struct {
	Client *caido.Client
	Mock   *MockHandler
	Server *httptest.Server
}

func NewTestEnv(t *testing.T) *TestEnv {
	t.Helper()
	mock := NewMockHandler()
	server := httptest.NewServer(mock)
	t.Cleanup(server.Close)

	client, err := caido.NewClient(caido.Options{
		URL:  server.URL,
		Auth: caido.PATAuth("test-token"),
	})
	if err != nil {
		t.Fatalf("failed to create test client: %v", err)
	}

	return &TestEnv{
		Client: client,
		Mock:   mock,
		Server: server,
	}
}
