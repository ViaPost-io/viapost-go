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
	untypedConst            = regexp.MustCompile(`(?m)^([ \t]+)([[:alnum:]_]+):\r?\n([ \t]+)const: ([^\r\n]+)[\t ]*$`)
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
	// OpenAPI 3.1 infers a scalar type from const, but ogen v1.24 emits invalid
	// encoders for untyped constants. Make that inferred type explicit only in
	// the temporary generation input.
	normalized = untypedConst.ReplaceAllStringFunc(normalized, func(match string) string {
		parts := untypedConst.FindStringSubmatch(match)
		value := strings.TrimSpace(parts[4])
		typ := "string"
		if value == "true" || value == "false" {
			typ = "boolean"
		} else if matched, _ := regexp.MatchString(`^-?[0-9]+$`, value); matched {
			typ = "integer"
		}
		return fmt.Sprintf("%s%s:\n%stype: %s\n%sconst: %s", parts[1], parts[2], parts[3], typ, parts[3], value)
	})
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
		`func encodePatchSegmentsIDRequest(
	req *UpdateSegmentRequest,
	r *http.Request,
) error {
	const contentType = "application/json"
`: `func encodePatchSegmentsIDRequest(
	req *UpdateSegmentRequest,
	r *http.Request,
) error {
	const contentType = "application/json"
	if err := validateUpdateSegmentRequest(req); err != nil {
		return err
	}
`,
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
	formattedContent, err := format.Source([]byte(content))
	if err != nil {
		return fmt.Errorf("format hardened generated JSON codecs: %w", err)
	}
	if err := os.WriteFile(path, formattedContent, 0o644); err != nil {
		return fmt.Errorf("write hardened generated JSON codecs: %w", err)
	}
	segmentUnionSource := `// Code generated by internal/generate from the public OpenAPI schemas; DO NOT EDIT.
package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/go-faster/jx"
	"github.com/google/uuid"
)

var segmentEventNamePattern = regexp.MustCompile(` + "`^[A-Za-z][A-Za-z0-9_-]*(\\.[A-Za-z0-9_-]+)*$`" + `)
var segmentTimestampPattern = regexp.MustCompile(` + "`^\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}(?:\\.\\d{1,9})?Z$`" + `)

// Validate checks the documented static or dynamic segment creation request.
func (s CreateSegmentRequest) Validate() error {
	if err := requireOnlyFields(map[string]jx.Raw(s), []string{"name", "description", "kind", "definition"}, []string{"name"}); err != nil { return err }
	if err := validateNonEmptyString(s["name"], "name"); err != nil { return err }
	if raw, ok := s["description"]; ok { if err := validateString(raw, "description"); err != nil { return err } }
	kind, present, err := optionalRawString(s, "kind")
	if err != nil { return err }
	if !present || kind == "static" {
		if _, ok := s["definition"]; ok { return fmt.Errorf("static segment request must not contain definition") }
		return nil
	}
	if kind != "dynamic" { return fmt.Errorf("segment request kind must be static or dynamic") }
	raw, ok := s["definition"]
	if !ok { return fmt.Errorf("dynamic segment request requires definition") }
	return validateSegmentDefinitionRaw(raw)
}

// Validate checks a segment response's static or dynamic variant without discarding its fields.
func (s Segment) Validate() error {
	values := map[string]jx.Raw(s)
	if err := requireOnlyFields(values, []string{"id", "name", "description", "kind", "definition", "contact_count", "created_at", "updated_at"}, []string{"id", "name", "description", "kind", "definition", "contact_count", "created_at", "updated_at"}); err != nil { return err }
	if err := validateUUID(values["id"], "id"); err != nil { return err }
	if err := validateNonEmptyString(values["name"], "name"); err != nil { return err }
	if err := validateNullableString(values["description"], "description"); err != nil { return err }
	if err := validateNonNegativeInteger(values["contact_count"], "contact_count"); err != nil { return err }
	if err := validateSegmentTimestamp(values["created_at"], "created_at"); err != nil { return err }
	if err := validateSegmentTimestamp(values["updated_at"], "updated_at"); err != nil { return err }
	kind, err := rawString(values, "kind")
	if err != nil { return err }
	if kind == "static" { if !rawIsNull(values["definition"]) { return fmt.Errorf("static segment definition must be null") }; return nil }
	if kind != "dynamic" { return fmt.Errorf("segment kind must be static or dynamic") }
	return validateSegmentDefinitionRaw(s["definition"])
}

// validateUpdateSegmentRequest supplements the generated scalar validation
// with the recursive SegmentDefinition one that ogen cannot emit for this union.
func validateUpdateSegmentRequest(s *UpdateSegmentRequest) error {
	if s == nil { return fmt.Errorf("segment update must not be nil") }
	if err := s.Validate(); err != nil { return err }
	if !s.Name.Set && !s.Description.Set && !s.Definition.Set { return fmt.Errorf("segment update requires at least one field") }
	if s.Definition.Set { return s.Definition.Value.Validate() }
	return nil
}

// Validate checks the all/any group union and its bounded recursive shape.
func (s SegmentDefinition) Validate() error {
	encoder := jx.Encoder{}
	s.Encode(&encoder)
	return validateSegmentDefinitionRaw(jx.Raw(encoder.Bytes()))
}

func validateSegmentDefinitionRaw(raw jx.Raw) error {
	predicates := 0
	return validateSegmentRule(raw, 0, true, &predicates)
}

func validateSegmentRule(raw jx.Raw, depth int, root bool, predicates *int) error {
	var node map[string]json.RawMessage
	if err := json.Unmarshal(raw, &node); err != nil { return fmt.Errorf("segment definition must be an object: %w", err) }
	if root {
		if len(node) != 1 { return fmt.Errorf("segment definition root must select all or any") }
		if _, all := node["all"]; !all { if _, any := node["any"]; !any { return fmt.Errorf("segment definition root must be an all or any group") } }
	}
	if _, all := node["all"]; !all { if _, any := node["any"]; !any {
		if root { return fmt.Errorf("segment definition root must be an all or any group") }
		*predicates++
		if *predicates > 100 { return fmt.Errorf("segment definition exceeds 100 predicates") }
		return validateSegmentPredicate(raw)
	} }
	if len(node) != 1 { return fmt.Errorf("segment group must select exactly one variant") }
	for key, value := range node {
		if key == "all" || key == "any" {
			if depth >= 4 { return fmt.Errorf("segment definition exceeds maximum group depth") }
			var children []json.RawMessage
			if err := json.Unmarshal(value, &children); err != nil || len(children) == 0 || len(children) > 25 { return fmt.Errorf("segment %s group must contain 1 to 25 rules", key) }
			for _, child := range children { if err := validateSegmentRule(jx.Raw(child), depth+1, false, predicates); err != nil { return err } }
			return nil
		}
		return fmt.Errorf("invalid segment group")
	}
	return fmt.Errorf("invalid segment rule")
}

func validateSegmentPredicate(raw jx.Raw) error {
	var values map[string]json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil { return fmt.Errorf("segment predicate must be an object: %w", err) }
	if _, group := values["all"]; group { return fmt.Errorf("segment predicate cannot be a group") }
	if _, group := values["any"]; group { return fmt.Errorf("segment predicate cannot be a group") }
	if rawField, ok := values["field"]; ok {
		var field string
		if err := json.Unmarshal(rawField, &field); err != nil { return fmt.Errorf("segment predicate field must be a string") }
		switch field {
		case "email", "first_name", "last_name":
			operator, err := predicateString(values, "operator"); if err != nil { return err }
			if oneOf(operator, "eq", "neq", "contains", "starts_with", "ends_with") { return validatePredicateFields(values, []string{"field", "operator", "value"}, func() error { return validateString(jx.Raw(values["value"]), "value") }) }
			if oneOf(operator, "is_set", "is_not_set") { return validatePredicateFields(values, []string{"field", "operator"}, nil) }
			return fmt.Errorf("invalid text predicate operator %q", operator)
		case "subscribed":
			return validatePredicateFields(values, []string{"field", "operator", "value"}, func() error { if operator, err := predicateString(values, "operator"); err != nil || operator != "eq" { return fmt.Errorf("subscribed predicate operator must be eq") }; if rawIsNull(jx.Raw(values["value"])) { return fmt.Errorf("subscribed predicate value must be boolean") }; var value bool; if err := json.Unmarshal(values["value"], &value); err != nil { return fmt.Errorf("subscribed predicate value must be boolean") }; return nil })
		case "created_at":
			return validatePredicateFields(values, []string{"field", "operator", "value"}, func() error { if operator, err := predicateString(values, "operator"); err != nil || !oneOf(operator, "before", "after") { return fmt.Errorf("created_at predicate operator must be before or after") }; return validateSegmentTimestamp(jx.Raw(values["value"]), "value") })
		case "property":
			operator, err := predicateString(values, "operator"); if err != nil { return err }
			if oneOf(operator, "eq", "neq") { return validatePredicateFields(values, []string{"field", "key", "operator", "value"}, func() error { if err := validateBoundedString(jx.Raw(values["key"]), "key", 1, 128); err != nil { return err }; return validateScalar(jx.Raw(values["value"]), "value") }) }
			if oneOf(operator, "exists", "not_exists") { return validatePredicateFields(values, []string{"field", "key", "operator"}, func() error { return validateBoundedString(jx.Raw(values["key"]), "key", 1, 128) }) }
			return fmt.Errorf("invalid property predicate operator %q", operator)
		default:
			return fmt.Errorf("invalid segment predicate field %q", field)
		}
	}
	return validatePredicateFields(values, []string{"event_name", "operator", "within_days"}, func() error { if err := validateBoundedString(jx.Raw(values["event_name"]), "event_name", 1, 120); err != nil { return err }; event, _ := rawString(map[string]jx.Raw{"event_name": jx.Raw(values["event_name"])}, "event_name"); if strings.HasPrefix(event, "viapost:") || !segmentEventNamePattern.MatchString(event) { return fmt.Errorf("event_name is invalid") }; operator, err := predicateString(values, "operator"); if err != nil || operator != "occurred" { return fmt.Errorf("custom event predicate operator must be occurred") }; var days int; if err := json.Unmarshal(values["within_days"], &days); err != nil || days < 1 || days > 90 { return fmt.Errorf("within_days must be an integer from 1 to 90") }; return nil })
}

func validatePredicateFields(values map[string]json.RawMessage, allowed []string, check func() error) error { if err := requireOnlyJSONFields(values, allowed, allowed); err != nil { return err }; if check != nil { return check() }; return nil }
func predicateString(values map[string]json.RawMessage, key string) (string, error) { raw, ok := values[key]; if !ok || rawIsNull(jx.Raw(raw)) { return "", fmt.Errorf("missing or null segment predicate field %q", key) }; var value string; if err := json.Unmarshal(raw, &value); err != nil { return "", fmt.Errorf("segment predicate field %q must be a string", key) }; return value, nil }
func rawIsNull(raw jx.Raw) bool { return bytes.Equal(bytes.TrimSpace(raw), []byte("null")) }
func rawString(values map[string]jx.Raw, key string) (string, error) { raw, ok := values[key]; if !ok || rawIsNull(raw) { return "", fmt.Errorf("missing or null segment field %q", key) }; var value string; if err := json.Unmarshal(raw, &value); err != nil { return "", fmt.Errorf("segment field %q must be a string", key) }; return value, nil }
func optionalRawString(values map[string]jx.Raw, key string) (string, bool, error) { if _, ok := values[key]; !ok { return "", false, nil }; value, err := rawString(values, key); return value, true, err }
func validateString(raw jx.Raw, key string) error { if rawIsNull(raw) { return fmt.Errorf("segment field %q must be a string", key) }; var value string; if err := json.Unmarshal(raw, &value); err != nil { return fmt.Errorf("segment field %q must be a string", key) }; return nil }
func validateNonEmptyString(raw jx.Raw, key string) error { return validateBoundedString(raw, key, 1, 0) }
func validateBoundedString(raw jx.Raw, key string, minimum, maximum int) error { var value string; if err := json.Unmarshal(raw, &value); err != nil { return fmt.Errorf("segment field %q must be a string", key) }; if len(value) < minimum || (maximum > 0 && len(value) > maximum) { return fmt.Errorf("segment field %q has invalid length", key) }; return nil }
func validateNullableString(raw jx.Raw, key string) error { if rawIsNull(raw) { return nil }; return validateString(raw, key) }
func validateUUID(raw jx.Raw, key string) error { if rawIsNull(raw) { return fmt.Errorf("segment field %q must be a UUID", key) }; var value string; if err := json.Unmarshal(raw, &value); err != nil { return fmt.Errorf("segment field %q must be a UUID", key) }; if _, err := uuid.Parse(value); err != nil { return fmt.Errorf("segment field %q must be a UUID", key) }; return nil }
func validateNonNegativeInteger(raw jx.Raw, key string) error { if rawIsNull(raw) { return fmt.Errorf("segment field %q must be a non-negative integer", key) }; var value int64; if err := json.Unmarshal(raw, &value); err != nil || value < 0 { return fmt.Errorf("segment field %q must be a non-negative integer", key) }; return nil }
func validateSegmentTimestamp(raw jx.Raw, key string) error { var value string; if err := json.Unmarshal(raw, &value); err != nil { return fmt.Errorf("segment field %q must be an RFC3339 UTC timestamp", key) }; if _, err := time.Parse(time.RFC3339Nano, value); err != nil || !segmentTimestampPattern.MatchString(value) { return fmt.Errorf("segment field %q must be an RFC3339 UTC timestamp", key) }; return nil }
func validateScalar(raw jx.Raw, key string) error { var value any; decoder := json.NewDecoder(bytes.NewReader(raw)); decoder.UseNumber(); if err := decoder.Decode(&value); err != nil { return fmt.Errorf("segment field %q must be scalar", key) }; switch value.(type) { case string, bool, json.Number: return nil; default: return fmt.Errorf("segment field %q must be string, number, or boolean", key) } }
func requireOnlyFields(values map[string]jx.Raw, allowed, required []string) error { raw := map[string]json.RawMessage{}; for key, value := range values { raw[key] = json.RawMessage(value) }; return requireOnlyJSONFields(raw, allowed, required) }
func requireOnlyJSONFields(values map[string]json.RawMessage, allowed, required []string) error { permitted := map[string]bool{}; for _, key := range allowed { permitted[key] = true }; for key := range values { if !permitted[key] { return fmt.Errorf("unexpected segment field %q", key) } }; for _, key := range required { if _, ok := values[key]; !ok { return fmt.Errorf("missing segment field %q", key) } }; return nil }
func oneOf(value string, values ...string) bool { for _, candidate := range values { if value == candidate { return true } }; return false }
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
