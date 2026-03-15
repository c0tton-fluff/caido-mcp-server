package auth

import "testing"

func TestNormalizeAuthWebSocketEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		want     string
	}{
		{
			name:     "upgrades legacy websocket path",
			endpoint: "ws://127.0.0.1:8080/ws",
			want:     "ws://127.0.0.1:8080/ws/graphql",
		},
		{
			name:     "keeps graphql websocket path",
			endpoint: "wss://caido.example/ws/graphql",
			want:     "wss://caido.example/ws/graphql",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeAuthWebSocketEndpoint(tt.endpoint)
			if got != tt.want {
				t.Fatalf("normalizeAuthWebSocketEndpoint() = %q, want %q", got, tt.want)
			}
		})
	}
}
