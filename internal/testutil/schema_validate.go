package testutil

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/validator"
)

// This file gives MockHandler real GraphQL validation.
//
// Without it the mock keys canned responses off operationName and accepts
// any variables at all, so a handler that sends a field the Caido schema
// does not declare still passes its test. That is not a hypothetical: the
// tamper tools sent `matcher` to TamperOperationMethodUpdateInput for
// three sections and every test stayed green, while a live Caido 0.57.0
// rejected the call outright with
//
//	Invalid value for argument "input.section.requestMethod.operation.update",
//	unknown field "matcher" of type "TamperOperationMethodUpdateInput"
//
// The schema is read from the pinned sdk-go module rather than vendored
// here, so it cannot drift from the version the client actually generates
// against. Reading it costs one `go list` per test binary.

var (
	schemaOnce sync.Once
	schemaVal  *ast.Schema
	schemaErr  error
)

// loadCaidoSchema parses the GraphQL schema shipped by the pinned sdk-go
// module. Result is cached for the lifetime of the test binary.
func loadCaidoSchema() (*ast.Schema, error) {
	schemaOnce.Do(func() {
		dir, err := sdkModuleDir()
		if err != nil {
			schemaErr = err
			return
		}
		path := filepath.Join(dir, "graphql", "schema.graphql")
		src, err := os.ReadFile(path) //nolint:gosec // path derived from go list
		if err != nil {
			schemaErr = fmt.Errorf("read caido schema %s: %w", path, err)
			return
		}
		schemaVal, schemaErr = gqlparser.LoadSchema(&ast.Source{
			Name:  "schema.graphql",
			Input: string(src),
		})
	})
	return schemaVal, schemaErr
}

// sdkModuleDir resolves the on-disk directory of the pinned sdk-go module.
func sdkModuleDir() (string, error) {
	out, err := exec.Command(
		"go", "list", "-m", "-f", "{{.Dir}}",
		"github.com/caido-community/sdk-go",
	).Output()
	if err != nil {
		return "", fmt.Errorf("locate sdk-go module: %w", err)
	}
	dir := strings.TrimSpace(string(out))
	if dir == "" {
		return "", fmt.Errorf("sdk-go module directory is empty")
	}
	return dir, nil
}

// validateOperation checks a query and its variables against the Caido
// schema. It returns an error for an invalid document or for variables
// that fail coercion -- including unknown input-object fields and oneof
// violations, which is what the live server enforces.
func validateOperation(
	query string, variables map[string]any,
) error {
	schema, err := loadCaidoSchema()
	if err != nil {
		return err
	}

	// nil rules selects gqlparser's default rule set, matching the
	// behaviour of the deprecated LoadQuery.
	doc, gqlErr := gqlparser.LoadQueryWithRules(schema, query, nil)
	if gqlErr != nil {
		return fmt.Errorf("invalid query: %s", gqlErr.Error())
	}
	if len(doc.Operations) == 0 {
		return fmt.Errorf("query contains no operations")
	}

	if variables == nil {
		variables = map[string]any{}
	}
	for _, op := range doc.Operations {
		if _, err := validator.VariableValues(
			schema, op, variables,
		); err != nil {
			return fmt.Errorf("invalid variables: %s", err.Error())
		}
	}
	return nil
}
