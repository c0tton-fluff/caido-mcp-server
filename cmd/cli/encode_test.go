package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRunDecode_Base64Fallbacks(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"std_padded", "aGVsbG8=", "hello"},                    // Decodes via StdEncoding
		{"rawurl_unpadded", "-_-_ABA", "\xfb\xff\xbf\x00\x10"}, // -_ chars + unpadded: fails Std/URL/RawStd, only RawURL
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			old := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			err := runDecode(&cobra.Command{}, []string{"base64", c.in})

			w.Close()
			os.Stdout = old
			var buf bytes.Buffer
			io.Copy(&buf, r)

			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if strings.TrimSpace(buf.String()) != c.want {
				t.Fatalf("expected %q, got %q", c.want, strings.TrimSpace(buf.String()))
			}
		})
	}
}
