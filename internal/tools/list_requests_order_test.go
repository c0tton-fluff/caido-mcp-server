package tools

import (
	"strings"
	"testing"

	gen "github.com/caido-community/sdk-go/graphql"
)

// TestBuildRequestOrder covers the whole orderBy table. The MCP-level tests go
// through the tool schema, which rejects unknown values before the handler
// runs, so this is where the mapping and the handler-side error text are
// checked directly.
func TestBuildRequestOrder(t *testing.T) {
	type tc struct {
		name    string
		orderBy string
		order   string
		want    *gen.RequestResponseOrderInput
		wantErr string
	}
	tests := []tc{
		{
			name: "both empty leaves the query unchanged",
		},
		{
			name:  "direction alone sorts by id",
			order: "desc",
			want:  &gen.RequestResponseOrderInput{By: gen.RequestResponseOrderById, Ordering: gen.OrderingDesc},
		},
		{
			name:  "asc direction alone",
			order: "asc",
			want:  &gen.RequestResponseOrderInput{By: gen.RequestResponseOrderById, Ordering: gen.OrderingAsc},
		},
		{
			name:    "field alone sorts descending",
			orderBy: "host",
			want:    &gen.RequestResponseOrderInput{By: gen.RequestResponseOrderByHost, Ordering: gen.OrderingDesc},
		},
		{
			name:    "unknown orderBy",
			orderBy: "size",
			wantErr: "orderBy must be one of",
		},
		{
			name:    "unknown order",
			order:   "up",
			wantErr: "order must be one of",
		},
		{
			name:    "order is case sensitive",
			order:   "DESC",
			wantErr: "order must be one of",
		},
	}

	// Every table entry maps to its own Caido enum value, so a copy-paste
	// slip in the table shows up as two names resolving to the same enum.
	seen := make(map[string]string, len(requestOrderFields))
	for _, f := range requestOrderFields {
		if prev, dup := seen[string(f.by)]; dup {
			t.Errorf("orderBy %q and %q both map to %s", prev, f.name, f.by)
		}
		seen[string(f.by)] = f.name

		tests = append(tests, tc{
			name:    "maps " + f.name,
			orderBy: f.name,
			order:   "asc",
			want:    &gen.RequestResponseOrderInput{By: f.by, Ordering: gen.OrderingAsc},
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildRequestOrder(tt.orderBy, tt.order)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error, got %v", got)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error %q does not contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.want == nil {
				if got != nil {
					t.Fatalf("got %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("got nil, want %v", tt.want)
			}
			if *got != *tt.want {
				t.Errorf("got %+v, want %+v", *got, *tt.want)
			}
		})
	}
}
