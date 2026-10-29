package testutil

import (
	_ "embed"
	"fmt"
	"os/exec"
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
// The schema is the published Caido proxy schema (caido/schemas,
// schemas/proxy/schema.graphql) for the server version this repo targets,
// vendored here because sdk-go lags the server: its bundled schema stops at
// 0.58.0 and has no Pipeline sessions. Validating against the server's
// schema is what matters, since the server is what rejects a bad call.
// TestSDKOperationsMatchSchema keeps the generated sdk-go operations honest
// against the same file, so a server change that breaks them fails here.
// To bump: replace the file and CaidoSchemaVersion together.

// CaidoSchemaVersion is the Caido release the vendored schema comes from.
const CaidoSchemaVersion = "0.59.0"

//go:embed caido-schema-v0.59.0.graphql
var caidoSchemaSource string

var (
	schemaOnce sync.Once
	schemaVal  *ast.Schema
	schemaErr  error
)

// loadCaidoSchema parses the vendored Caido schema. Result is cached for
// the lifetime of the test binary.
func loadCaidoSchema() (*ast.Schema, error) {
	schemaOnce.Do(func() {
		schemaVal, schemaErr = gqlparser.LoadSchema(&ast.Source{
			Name:  "caido-schema-v" + CaidoSchemaVersion + ".graphql",
			Input: caidoSchemaSource,
		})
	})
	return schemaVal, schemaErr
}

// SDKModuleDir resolves the on-disk directory of the pinned sdk-go module.
func SDKModuleDir() (string, error) {
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

// ValidateDocument checks a GraphQL document (one or more operations and
// fragments) against the vendored Caido schema, without variables.
func ValidateDocument(query string) error {
	schema, err := loadCaidoSchema()
	if err != nil {
		return err
	}
	if _, gqlErr := gqlparser.LoadQueryWithRules(schema, query, nil); gqlErr != nil {
		return fmt.Errorf("invalid query: %s", gqlErr.Error())
	}
	return nil
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
