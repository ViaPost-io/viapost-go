// Package generate normalizes the public OpenAPI contract for ogen and invokes
// the pinned generator without fetching a remote schema.
package generate

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

//go:generate go run ../cmd/generate ../../openapi.yaml ../../api

var (
	unsupportedItems     = regexp.MustCompile(`(?m)^[\t ]*items: false[\t ]*\r?\n`)
	sessionReference     = regexp.MustCompile(`(?m)^[\t ]*- sessionCookie: \[\][\t ]*\r?\n`)
	csrfListReference    = regexp.MustCompile(`(?m)^[\t ]*- \$ref: '#/components/parameters/CsrfHeader'[\t ]*\r?\n`)
	csrfInlineParameters = regexp.MustCompile(`(?m)^[\t ]*parameters: \[\{ \$ref: '#/components/parameters/CsrfHeader' \}\][\t ]*\r?\n`)
	csrfComponent        = regexp.MustCompile(`(?ms)^    CsrfHeader:\r?\n.*?(^  securitySchemes:)`)
	sessionScheme        = regexp.MustCompile(`(?ms)^    sessionCookie:\r?\n.*?(^  schemas:)`)
)

const (
	canonicalUUIDSchema = "    UUID: { type: string, format: uuid }"
	uuidSchemaSentinel  = "    UUID: { type: string, format: viapost-canonical-uuid }"
)

func normalizeCodegenSpec(source []byte) []byte {
	normalized := string(append([]byte(nil), source...))
	// ogen gives every inline UUID schema the same generated helper names. Keep
	// the canonical UUID component typed and relax only those conflicting
	// anonymous schemas until the public contract references UUID consistently.
	normalized = strings.Replace(normalized, canonicalUUIDSchema, uuidSchemaSentinel, 1)
	normalized = strings.ReplaceAll(normalized, ", format: uuid", "")
	normalized = strings.Replace(normalized, uuidSchemaSentinel, canonicalUUIDSchema, 1)
	normalized = unsupportedItems.ReplaceAllString(normalized, "")
	normalized = sessionReference.ReplaceAllString(normalized, "")
	normalized = csrfListReference.ReplaceAllString(normalized, "")
	normalized = csrfInlineParameters.ReplaceAllString(normalized, "")
	normalized = csrfComponent.ReplaceAllString(normalized, "$1")
	normalized = sessionScheme.ReplaceAllString(normalized, "$1")
	return []byte(normalized)
}

// Generate writes the normalized contract to a temporary file and generates a
// complete client. The checked-in openapi.yaml remains the exact source of truth.
func Generate(sourcePath, targetPath string) error {
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		return fmt.Errorf("read OpenAPI contract: %w", err)
	}

	temporary, err := os.CreateTemp("", "viapost-openapi-codegen-*.yaml")
	if err != nil {
		return fmt.Errorf("create normalized OpenAPI contract: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()

	if _, err := temporary.Write(normalizeCodegenSpec(source)); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write normalized OpenAPI contract: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close normalized OpenAPI contract: %w", err)
	}

	configPath := filepath.Join(filepath.Dir(sourcePath), "ogen.yml")
	command := exec.Command("go", "tool", "ogen", "--config", configPath, "--target", targetPath, "--package", "api", "--clean", temporaryPath)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("generate client: %w", err)
	}
	return nil
}
