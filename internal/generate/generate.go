// Package generate normalizes the public OpenAPI contract for ogen and invokes
// the pinned generator without fetching a remote schema.
package generate

import (
	"bytes"
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
	unsupportedItems        = regexp.MustCompile(`(?m)^[\t ]*items: false[\t ]*\r?\n`)
	canonicalUUIDBlock      = regexp.MustCompile(`(?m)^    UUID:\r?\n      type: string\r?\n      format: uuid[\t ]*$`)
	uuidFormatLine          = regexp.MustCompile(`(?m)^[\t ]*format: uuid[\t ]*\r?\n`)
	sessionReference        = regexp.MustCompile(`(?m)^[\t ]*- sessionCookie: \[\][\t ]*\r?\n`)
	csrfListReference       = regexp.MustCompile(`(?m)^[\t ]*- \$ref: '#/components/parameters/CsrfHeader'[\t ]*\r?\n`)
	csrfInlineParameters    = regexp.MustCompile(`(?m)^[\t ]*parameters: \[\{ \$ref: '#/components/parameters/CsrfHeader' \}\][\t ]*\r?\n`)
	csrfComponent           = regexp.MustCompile(`(?ms)^    CsrfHeader:\r?\n.*?(^  securitySchemes:)`)
	sessionScheme           = regexp.MustCompile(`(?ms)^    sessionCookie:\r?\n.*?(^  schemas:)`)
	webhookDeliveryEvent    = regexp.MustCompile(`(?ms)^    WebhookDeliveryEventType:\r?\n      anyOf:\r?\n        - \$ref: '#/components/schemas/WebhookSubscribableEventType'\r?\n        - type: string\r?\n          const: webhook\.test[\t ]*$`)
	propertyComparisonValue = regexp.MustCompile(`(?m)^        value:\r?\n          type:\r?\n            - string\r?\n            - number\r?\n            - boolean\r?\n`)
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
	// ogen v1.24 does not support JSON Schema's multi-type form. The generated
	// client represents this documented scalar union as an unconstrained value.
	normalized = propertyComparisonValue.ReplaceAllString(normalized, "        value: {}\n")
	var err error
	normalized, err = rawSegmentObjectUnions(normalized)
	if err != nil {
		return nil, err
	}
	return []byte(normalized), nil
}

func rawSegmentObjectUnions(source string) (string, error) {
	for _, union := range []struct{ name, next string }{
		{"Segment", "StaticSegment"}, {"CreateSegmentRequest", "UpdateSegmentRequest"},
		{"SegmentDefinition", "AllGroupDepth1"}, {"SegmentRuleDepth1", "SegmentRuleDepth2"},
		{"SegmentRuleDepth2", "SegmentRuleDepth3"}, {"SegmentRuleDepth3", "LeafRule"},
		{"LeafRule", "TextAttributePredicate"},
	} {
		pattern := regexp.MustCompile(`(?ms)^    ` + regexp.QuoteMeta(union.name) + `:\r?\n(?:      description:.*\r?\n)?      oneOf:\r?\n.*?(^    ` + regexp.QuoteMeta(union.next) + `:)`)
		matches := pattern.FindAllStringIndex(source, -1)
		if len(matches) == 0 {
			continue
		}
		if len(matches) != 1 {
			return "", fmt.Errorf("expected exactly one %s object union, found %d", union.name, len(matches))
		}
		source = pattern.ReplaceAllString(source, "    "+union.name+":\n      type: object\n      additionalProperties: true\n$1")
	}
	return source, nil
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
	if err := hardenSensitiveResponses(targetPath); err != nil {
		return err
	}
	return addSegmentBoundaryValidation(targetPath)
}

func addSegmentBoundaryValidation(targetPath string) error {
	path := filepath.Join(targetPath, "oas_request_encoders_gen.go")
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read generated segment request encoders: %w", err)
	}
	replacements := map[string]string{
		`func encodePostSegmentsRequest(
	req CreateSegmentRequest,
	r *http.Request,
) error {
	const contentType = "application/json"
`: `func encodePostSegmentsRequest(
	req CreateSegmentRequest,
	r *http.Request,
) error {
	const contentType = "application/json"
	if err := req.Validate(); err != nil {
		return err
	}
`,
		`func encodePostSegmentsPreviewRequest(
	req *SegmentPreviewRequest,
	r *http.Request,
) error {
	const contentType = "application/json"
`: `func encodePostSegmentsPreviewRequest(
	req *SegmentPreviewRequest,
	r *http.Request,
) error {
	const contentType = "application/json"
	if err := req.Definition.Validate(); err != nil {
		return err
	}
`,
	}
	for original, replacement := range replacements {
		if bytes.Count(content, []byte(original)) != 1 {
			return fmt.Errorf("expected exactly one generated segment request encoder")
		}
		content = bytes.Replace(content, []byte(original), []byte(replacement), 1)
	}
	formatted, err := format.Source(content)
	if err != nil {
		return fmt.Errorf("format generated segment request encoders: %w", err)
	}
	if err := os.WriteFile(path, formatted, 0o644); err != nil {
		return fmt.Errorf("write generated segment request encoders: %w", err)
	}

	path = filepath.Join(targetPath, "oas_json_gen.go")
	content, err = os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read generated segment JSON codecs: %w", err)
	}
	original := `	}); err != nil {
		return errors.Wrap(err, "decode Segment")
	}

	return nil
}`
	replacement := `	}); err != nil {
		return errors.Wrap(err, "decode Segment")
	}
	if err := s.Validate(); err != nil {
		return errors.Wrap(err, "validate Segment")
	}

	return nil
}`
	if bytes.Count(content, []byte(original)) != 1 {
		return fmt.Errorf("expected exactly one generated Segment decoder")
	}
	content = bytes.Replace(content, []byte(original), []byte(replacement), 1)
	formatted, err = format.Source(content)
	if err != nil {
		return fmt.Errorf("format generated segment JSON codecs: %w", err)
	}
	if err := os.WriteFile(path, formatted, 0o644); err != nil {
		return fmt.Errorf("write generated segment JSON codecs: %w", err)
	}
	return nil
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
	// ogen v1.24 emits this newly added string const as Raw(string), while the
	// current jx API requires raw JSON bytes. Encode the documented const as a
	// JSON string instead.
	const dynamicMembershipRaw = `e.Raw("dynamic_segment_membership")`
	if strings.Count(content, dynamicMembershipRaw) != 1 {
		return fmt.Errorf("expected exactly one generated dynamic membership const codec")
	}
	content = strings.Replace(content, dynamicMembershipRaw, `e.Str("dynamic_segment_membership")`, 1)
	formattedContent, err := format.Source([]byte(content))
	if err != nil {
		return fmt.Errorf("format hardened generated JSON codecs: %w", err)
	}
	if err := os.WriteFile(path, formattedContent, 0o644); err != nil {
		return fmt.Errorf("write hardened generated JSON codecs: %w", err)
	}
	segmentUnionSource := `// Code generated by internal/generate from the public OpenAPI oneOf schemas; DO NOT EDIT.
package api

import (
	"encoding/json"
	"fmt"

	"github.com/go-faster/jx"
)

// Validate checks which documented oneOf variant a segment creation request represents.
func (s CreateSegmentRequest) Validate() error {
	if err := requireSegmentFields(map[string]jx.Raw(s), "name", "description", "kind", "definition"); err != nil { return err }
	name, err := rawString(s, "name"); if err != nil || name == "" { return fmt.Errorf("segment request requires non-empty name") }
	kind, present, err := optionalRawString(s, "kind"); if err != nil { return err }
	if !present || kind == "static" {
		if _, ok := s["definition"]; ok { return fmt.Errorf("static segment request must not contain definition") }
		return nil
	}
	if kind != "dynamic" { return fmt.Errorf("segment request kind must be static or dynamic") }
	raw, ok := s["definition"]; if !ok { return fmt.Errorf("dynamic segment request requires definition") }
	return validateSegmentDefinitionRaw(raw)
}

// Validate checks a segment response's static or dynamic variant without discarding its fields.
func (s Segment) Validate() error {
	if err := requireSegmentFields(map[string]jx.Raw(s), "id", "name", "description", "kind", "definition", "contact_count", "created_at", "updated_at"); err != nil { return err }
	kind, err := rawString(s, "kind"); if err != nil { return err }
	if kind == "static" { if !rawIsNull(s["definition"]) { return fmt.Errorf("static segment definition must be null") }; return nil }
	if kind != "dynamic" { return fmt.Errorf("segment kind must be static or dynamic") }
	return validateSegmentDefinitionRaw(s["definition"])
}

// Validate checks the all/any group union and its bounded recursive shape.
func (s SegmentDefinition) Validate() error {
	encoder := jx.Encoder{}
	s.Encode(&encoder)
	return validateSegmentDefinitionRaw(jx.Raw(encoder.Bytes()))
}

func validateSegmentDefinitionRaw(raw jx.Raw) error { return validateSegmentRule(raw, 0, true) }

func validateSegmentRule(raw jx.Raw, depth int, root bool) error {
	var node map[string]json.RawMessage
	if err := json.Unmarshal(raw, &node); err != nil { return fmt.Errorf("segment definition must be an object: %w", err) }
	if len(node) != 1 { return fmt.Errorf("segment rule must select exactly one variant") }
	for key, value := range node {
		if key == "all" || key == "any" {
			if depth >= 3 { return fmt.Errorf("segment definition exceeds maximum group depth") }
			var children []json.RawMessage
			if err := json.Unmarshal(value, &children); err != nil || len(children) == 0 || len(children) > 25 { return fmt.Errorf("segment %s group must contain 1 to 25 rules", key) }
			for _, child := range children { if err := validateSegmentRule(jx.Raw(child), depth+1, false); err != nil { return err } }
			return nil
		}
		if root { return fmt.Errorf("segment definition root must be an all or any group") }
		if !segmentLeafVariant(key) { return fmt.Errorf("unknown segment predicate variant %q", key) }
		var predicate map[string]json.RawMessage
		if err := json.Unmarshal(value, &predicate); err != nil || len(predicate) == 0 { return fmt.Errorf("segment predicate %q must be a non-empty object", key) }
		return nil
	}
	return fmt.Errorf("invalid segment rule")
}

func segmentLeafVariant(key string) bool { switch key { case "text_attribute", "text_presence", "subscribed", "created_at", "property_value", "property_presence", "custom_event": return true }; return false }
func rawIsNull(raw jx.Raw) bool { return string(raw) == "null" }
func rawString(values map[string]jx.Raw, key string) (string, error) { raw, ok := values[key]; if !ok { return "", fmt.Errorf("missing segment field %q", key) }; var value string; if err := json.Unmarshal(raw, &value); err != nil { return "", fmt.Errorf("segment field %q must be a string", key) }; return value, nil }
func optionalRawString(values map[string]jx.Raw, key string) (string, bool, error) { raw, ok := values[key]; if !ok { return "", false, nil }; var value string; if err := json.Unmarshal(raw, &value); err != nil { return "", true, fmt.Errorf("segment field %q must be a string", key) }; return value, true, nil }
func requireSegmentFields(values map[string]jx.Raw, allowed ...string) error { permitted := map[string]bool{}; for _, key := range allowed { permitted[key] = true }; for key := range values { if !permitted[key] { return fmt.Errorf("unexpected segment field %q", key) } }; return nil }
`
	formattedSegmentUnionSource, err := format.Source([]byte(segmentUnionSource))
	if err != nil {
		return fmt.Errorf("format generated segment union support: %w", err)
	}
	if err := os.WriteFile(filepath.Join(targetPath, "segment_unions.go"), formattedSegmentUnionSource, 0o644); err != nil {
		return fmt.Errorf("write generated segment union support: %w", err)
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
