package httputil

import (
	"strings"
	"testing"
)

func TestNormalizeCRLF_LiteralEscapes(t *testing.T) {
	input := `GET / HTTP/1.1\r\nHost: example.com\r\n\r\n`
	got := NormalizeCRLF(input)
	want := "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestNormalizeCRLF_AlreadyCorrect(t *testing.T) {
	input := "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"
	got := NormalizeCRLF(input)
	if got != input {
		t.Fatalf("got %q, want %q", got, input)
	}
}

func TestNormalizeCRLF_BodyWithLiteralEscapes(t *testing.T) {
	// A raw request whose JSON body contains the literal four-character
	// sequence \r\n (valid JSON escape) must NOT have those bytes converted
	// to real CRLF. The input already has real CRLF in the headers, so
	// escape interpretation is skipped entirely.
	body := `{"m":"hello\r\nworld"}`
	input := "POST / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 22\r\n\r\n" + body
	got := NormalizeCRLF(input)
	if !strings.Contains(got, body) {
		t.Fatalf("NormalizeCRLF corrupted body with literal escapes:\n got %q", got)
	}
}

func TestNormalizeCRLF_MissingTrailing(t *testing.T) {
	input := "GET / HTTP/1.1\r\nHost: example.com"
	got := NormalizeCRLF(input)
	want := "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestNormalizeCRLF_SingleTrailingCRLF(t *testing.T) {
	input := "GET / HTTP/1.1\r\nHost: example.com\r\n"
	got := NormalizeCRLF(input)
	want := "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestParseHostHeader_Found(t *testing.T) {
	raw := "GET / HTTP/1.1\nHost: example.com\nAccept: */*\n"
	got := ParseHostHeader(raw)
	if got != "example.com" {
		t.Fatalf("got %q, want %q", got, "example.com")
	}
}

func TestParseHostHeader_CaseInsensitive(t *testing.T) {
	raw := "GET / HTTP/1.1\nhost: EXAMPLE.COM\n"
	got := ParseHostHeader(raw)
	if got != "EXAMPLE.COM" {
		t.Fatalf("got %q, want %q", got, "EXAMPLE.COM")
	}
}

func TestParseHostHeader_NotFound(t *testing.T) {
	raw := "GET / HTTP/1.1\nAccept: */*\n"
	got := ParseHostHeader(raw)
	if got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestParseHostHeader_EmptyInput(t *testing.T) {
	got := ParseHostHeader("")
	if got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

// TestNormalizeCRLF_BodyUntouched pins that only the header section is
// normalized. The body is sent byte for byte: appending CRLF after it or
// rewriting its LFs changes what the server receives and breaks
// Content-Length.
func TestNormalizeCRLF_BodyUntouched(t *testing.T) {
	tests := []struct {
		name, input, want string
	}{
		{
			name:  "no trailing CRLF appended after a body",
			input: "POST /cart HTTP/1.1\r\nHost: a.test\r\nContent-Length: 5\r\n\r\nqty=2",
			want:  "POST /cart HTTP/1.1\r\nHost: a.test\r\nContent-Length: 5\r\n\r\nqty=2",
		},
		{
			name:  "LF headers normalized, LF body kept",
			input: "POST / HTTP/1.1\nHost: a.test\nContent-Length: 7\n\n{\n\"a\"}\n",
			want:  "POST / HTTP/1.1\r\nHost: a.test\r\nContent-Length: 7\r\n\r\n{\n\"a\"}\n",
		},
		{
			name:  "body containing a blank line is not split again",
			input: "POST / HTTP/1.1\r\nHost: a.test\r\n\r\npart1\r\n\r\npart2",
			want:  "POST / HTTP/1.1\r\nHost: a.test\r\n\r\npart1\r\n\r\npart2",
		},
		{
			name:  "mixed endings in headers",
			input: "GET / HTTP/1.1\r\nHost: a.test\nX: y\r\n\nbody",
			want:  "GET / HTTP/1.1\r\nHost: a.test\r\nX: y\r\n\r\nbody",
		},
		{
			name:  "empty body after terminator stays empty",
			input: "GET / HTTP/1.1\nHost: a.test\n\n",
			want:  "GET / HTTP/1.1\r\nHost: a.test\r\n\r\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeCRLF(tt.input); got != tt.want {
				t.Fatalf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}
