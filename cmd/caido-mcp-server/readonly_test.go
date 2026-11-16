package main

import (
	"testing"

	"github.com/spf13/cobra"
)

// TestBoolOption covers how serve decides a boolean option is on, for each
// option. An unparseable environment value must be an error: the
// operator asked for a restriction, and falling back to the unrestricted
// server would be the unsafe reading of a typo.
func TestBoolOption(t *testing.T) {
	for _, r := range []struct{ flag, env string }{
		{"read-only", "CAIDO_MCP_READ_ONLY"},
		{"enforce-scope", "CAIDO_MCP_ENFORCE_SCOPE"},
		{"compact", "CAIDO_MCP_COMPACT"},
	} {
		t.Run(r.flag, func(t *testing.T) { testBoolOption(t, r.flag, r.env) })
	}
}

func testBoolOption(t *testing.T, flag, env string) {
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
			t.Setenv(env, tt.env)
			cmd := &cobra.Command{}
			cmd.Flags().Bool(flag, false, "")
			if tt.flag {
				if err := cmd.Flags().Set(flag, "true"); err != nil {
					t.Fatal(err)
				}
			}
			got, err := boolOption(cmd, flag, env)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("boolOption = %v, want %v", got, tt.want)
			}
		})
	}
}
