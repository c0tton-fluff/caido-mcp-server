package main

import (
	"testing"

	"github.com/spf13/cobra"
)

// TestReadOnlyMode covers how serve decides to run read-only. An
// unparseable CAIDO_MCP_READ_ONLY must be an error: the operator asked for
// a restriction, and falling back to the full tool set would be the unsafe
// reading of a typo.
func TestReadOnlyMode(t *testing.T) {
	tests := []struct {
		name    string
		flag    bool
		env     string
		want    bool
		wantErr bool
	}{
		{name: "default is off"},
		{name: "flag", flag: true, want: true},
		{name: "env 1", env: "1", want: true},
		{name: "env true", env: "true", want: true},
		{name: "env 0", env: "0"},
		{name: "env false", env: "false"},
		{name: "flag wins over env off", flag: true, env: "0", want: true},
		{name: "env typo is an error", env: "yes", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CAIDO_MCP_READ_ONLY", tt.env)
			cmd := &cobra.Command{}
			cmd.Flags().Bool("read-only", false, "")
			if tt.flag {
				if err := cmd.Flags().Set("read-only", "true"); err != nil {
					t.Fatal(err)
				}
			}
			got, err := readOnlyMode(cmd)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("readOnlyMode = %v, want %v", got, tt.want)
			}
		})
	}
}
