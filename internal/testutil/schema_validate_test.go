package testutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSDKOperationsMatchSchema validates every operation the pinned sdk-go
// generates against the vendored server schema. sdk-go is generated from
// its own (older) schema copy, so this is the check that tells us a Caido
// release removed or renamed something the client still sends.
func TestSDKOperationsMatchSchema(t *testing.T) {
	dir, err := SDKModuleDir()
	if err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(dir, "graphql", "operations", "*.graphql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no sdk-go operation files under %s (err %v)", dir, err)
	}
	var doc strings.Builder
	for _, f := range files {
		b, err := os.ReadFile(f) //nolint:gosec // path from go list + glob
		if err != nil {
			t.Fatal(err)
		}
		doc.Write(b)
		doc.WriteString("\n")
	}
	if err := ValidateDocument(doc.String()); err != nil {
		t.Fatalf("sdk-go operations no longer match Caido %s: %v", CaidoSchemaVersion, err)
	}
}

// TestValidateDocumentRejectsRemovedField is the negative control for the
// test above: a field Caido 0.59 removed must fail, or the check is a no-op.
func TestValidateDocumentRejectsRemovedField(t *testing.T) {
	err := ValidateDocument(`query X { instanceSettings { aiProviders { anthropic { apiKey } } } }`)
	if err == nil || !strings.Contains(err.Error(), "aiProviders") {
		t.Fatalf("want an aiProviders validation error, got %v", err)
	}
}
