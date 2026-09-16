// Package generate normalizes the public OpenAPI contract for ogen and invokes
// the pinned generator without fetching a remote schema.
package generate

import (
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

//go:generate go run ../cmd/generate ../../openapi.yaml ../../api

var (
	unsupportedItems     = regexp.MustCompile(`(?m)^[\t ]*items: false[\t ]*\r?\n`)
	canonicalUUIDBlock   = regexp.MustCompile(`(?m)^    UUID:\r?\n      type: string\r?\n      format: uuid[\t ]*$`)
	uuidFormatLine       = regexp.MustCompile(`(?m)^[\t ]*format: uuid[\t ]*\r?\n`)
	sessionReference     = regexp.MustCompile(`(?m)^[\t ]*- sessionCookie: \[\][\t ]*\r?\n`)
	csrfListReference    = regexp.MustCompile(`(?m)^[\t ]*- \$ref: '#/components/parameters/CsrfHeader'[\t ]*\r?\n`)
	csrfInlineParameters = regexp.MustCompile(`(?m)^[\t ]*parameters: \[\{ \$ref: '#/components/parameters/CsrfHeader' \}\][\t ]*\r?\n`)
	csrfComponent        = regexp.MustCompile(`(?ms)^    CsrfHeader:\r?\n.*?(^  securitySchemes:)`)
	sessionScheme        = regexp.MustCompile(`(?ms)^    sessionCookie:\r?\n.*?(^  schemas:)`)
	webhookDeliveryEvent = regexp.MustCompile(`(?ms)^    WebhookDeliveryEventType:\r?\n      anyOf:\r?\n        - \$ref: '#/components/schemas/WebhookSubscribableEventType'\r?\n        - type: string\r?\n          const: webhook\.test[\t ]*$`)
)

const (
	canonicalUUIDSchema = "    UUID:\n      type: string\n      format: uuid"
	uuidSchemaSentinel  = "    UUID:\n      type: string\n      format: viapost-canonical-uuid"
)

func normalizeCodegenSpec(source []byte) ([]byte, error) {
	normalized := string(append([]byte(nil), source...))
	// ogen gives every inline UUID schema the same generated helper names. Keep
	// the canonical UUID component typed and relax only those conflicting
	// anonymous schemas until the public contract references UUID consistently.
	if matches := canonicalUUIDBlock.FindAllStringIndex(normalized, -1); len(matches) != 1 {
		return nil, fmt.Errorf("expected exactly one canonical UUID schema, found %d", len(matches))
	}
	normalized = canonicalUUIDBlock.ReplaceAllString(normalized, uuidSchemaSentinel)
	normalized = strings.ReplaceAll(normalized, ", format: uuid", "")
	normalized = uuidFormatLine.ReplaceAllString(normalized, "")
	normalized = strings.Replace(normalized, uuidSchemaSentinel, canonicalUUIDSchema, 1)
	normalized = unsupportedItems.ReplaceAllString(normalized, "")
	normalized = sessionReference.ReplaceAllString(normalized, "")
	normalized = csrfListReference.ReplaceAllString(normalized, "")
	normalized = csrfInlineParameters.ReplaceAllString(normalized, "")
	normalized = csrfComponent.ReplaceAllString(normalized, "$1")
	normalized = sessionScheme.ReplaceAllString(normalized, "$1")
	// ogen v1.24 cannot represent the union between a referenced string enum
	// and one additional string literal. Keep the response forward-compatible
	// as a plain string in generated code; the versioned contract still retains
	// the exact anyOf constraints for documentation and drift checks.
	normalized = webhookDeliveryEvent.ReplaceAllString(normalized, "    WebhookDeliveryEventType:\n      type: string")
	return []byte(normalized), nil
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

	normalized, err := normalizeCodegenSpec(source)
	if err != nil {
		_ = temporary.Close()
		return fmt.Errorf("normalize OpenAPI contract: %w", err)
	}
	if _, err := temporary.Write(normalized); err != nil {
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
	return hardenSensitiveResponses(targetPath)
}

func hardenSensitiveResponses(targetPath string) error {
	path := filepath.Join(targetPath, "oas_json_gen.go")
	generated, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read generated JSON codecs: %w", err)
	}
	content := string(generated)
	replacements := map[string]string{
		`func (s *CreateWebhookResponse) MarshalJSON() ([]byte, error) {
	e := jx.Encoder{}
	s.Encode(&e)
	return e.Bytes(), nil
}`: `func (s CreateWebhookResponse) MarshalJSON() ([]byte, error) {
	s.Secret = "[REDACTED]"
	e := jx.Encoder{}
	s.Encode(&e)
	return e.Bytes(), nil
}`,
		`func (s *RotateWebhookSecretResponse) MarshalJSON() ([]byte, error) {
	e := jx.Encoder{}
	s.Encode(&e)
	return e.Bytes(), nil
}`: `func (s RotateWebhookSecretResponse) MarshalJSON() ([]byte, error) {
	if s.Secret.Set {
		s.Secret.Value = "[REDACTED]"
	}
	e := jx.Encoder{}
	s.Encode(&e)
	return e.Bytes(), nil
}`,
	}
	for original, hardened := range replacements {
		if strings.Count(content, original) != 1 {
			return fmt.Errorf("expected exactly one generated sensitive codec to harden")
		}
		content = strings.Replace(content, original, hardened, 1)
	}
	formattedContent, err := format.Source([]byte(content))
	if err != nil {
		return fmt.Errorf("format hardened generated JSON codecs: %w", err)
	}
	if err := os.WriteFile(path, formattedContent, 0o644); err != nil {
		return fmt.Errorf("write hardened generated JSON codecs: %w", err)
	}
	sensitiveSource := `package api

import "log/slog"

const redactedSensitiveValue = "[REDACTED]"

func (s CreateWebhookResponse) String() string { return "CreateWebhookResponse{Secret:[REDACTED]}" }
func (s CreateWebhookResponse) GoString() string { return s.String() }
func (s CreateWebhookResponse) LogValue() slog.Value {
	return slog.GroupValue(slog.String("secret", redactedSensitiveValue))
}

func (s RotateWebhookSecretResponse) String() string { return "RotateWebhookSecretResponse{Secret:[REDACTED]}" }
func (s RotateWebhookSecretResponse) GoString() string { return s.String() }
func (s RotateWebhookSecretResponse) LogValue() slog.Value {
	return slog.GroupValue(slog.String("secret", redactedSensitiveValue))
}

func (c Client) String() string { return "ViaPostOpenAPIClient{Security:[REDACTED]}" }
func (c Client) GoString() string { return c.String() }
func (c Client) LogValue() slog.Value {
	return slog.GroupValue(slog.String("security", redactedSensitiveValue))
}
`
	formattedSensitiveSource, err := format.Source([]byte(sensitiveSource))
	if err != nil {
		return fmt.Errorf("format generated sensitive representations: %w", err)
	}
	if err := os.WriteFile(filepath.Join(targetPath, "sensitive.go"), formattedSensitiveSource, 0o644); err != nil {
		return fmt.Errorf("write generated sensitive representations: %w", err)
	}
	return nil
}
