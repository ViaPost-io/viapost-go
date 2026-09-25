package generate

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ghodss/yaml"
)

func TestNormalizeCodegenSpec_PreservesSourceAndRemovesUnsupportedConstraints(t *testing.T) {
	source := []byte(`openapi: 3.1.1
schema:
  id: { type: string, format: uuid }
  expanded_id:
    type: string
    format: uuid
  status: { type: string, enum: [ready, failed] }
  items: false
components:
  schemas:
    UUID:
      type: string
      format: uuid
security:
  - bearerApiKey: []
  - sessionCookie: []
parameters: [{ $ref: '#/components/parameters/CsrfHeader' }]
    WebhookDeliveryEventType:
      anyOf:
        - $ref: '#/components/schemas/WebhookSubscribableEventType'
        - type: string
          const: webhook.test
`)

	normalized, err := normalizeCodegenSpec(source)
	if err != nil {
		t.Fatalf("normalizeCodegenSpec() error = %v", err)
	}
	got := string(normalized)

	for _, unwanted := range []string{"id: { type: string, format: uuid }", "expanded_id:\n    type: string\n    format: uuid", "items: false", "sessionCookie", "CsrfHeader"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("normalized schema still contains %q:\n%s", unwanted, got)
		}
	}
	if !strings.Contains(got, canonicalUUIDSchema) {
		t.Fatalf("normalized schema lost canonical UUID format:\n%s", got)
	}
	if !strings.Contains(got, "enum: [ready, failed]") {
		t.Fatalf("normalized schema lost supported enum constraints:\n%s", got)
	}
	if !strings.Contains(got, "openapi: 3.1.1") {
		t.Fatalf("normalized schema lost unrelated content:\n%s", got)
	}
	if !strings.Contains(got, "WebhookDeliveryEventType:\n      type: string") || strings.Contains(got, "const: webhook.test") {
		t.Fatalf("normalized schema retained unsupported webhook event union:\n%s", got)
	}
	if !strings.Contains(string(source), "items: false") {
		t.Fatal("normalization mutated the source buffer")
	}
}

func TestNormalizeCodegenSpec_RelaxesOnlySendCustomEventSelectionAnyOf(t *testing.T) {
	source := []byte(`openapi: 3.1.1
info: { title: test, version: 1.0.0 }
paths: {}
components:
  schemas:
    UUID:
      type: string
      format: uuid
    SendCustomEventRequest:
      type: object
      additionalProperties: false
      required: [event]
      description: Exactly one effective identifier.
      anyOf:
        - required: [contact_id]
          properties:
            contact_id: { $ref: '#/components/schemas/UUID' }
            email:
              anyOf:
                - { type: 'null' }
                - { type: string, const: '' }
        - required: [email]
          properties:
            contact_id: { type: 'null' }
            email: { type: string, format: email }
      properties:
        event: { type: string, maxLength: 120 }
        contact_id:
          anyOf:
            - { $ref: '#/components/schemas/UUID' }
            - { type: 'null' }
        email:
          anyOf:
            - { type: string }
            - { type: 'null' }
        payload: { type: object, additionalProperties: true }
`)

	normalized, err := normalizeCodegenSpec(source)
	if err != nil {
		t.Fatalf("normalizeCodegenSpec() error = %v", err)
	}
	got := string(normalized)

	if strings.Contains(got, "      anyOf:\n        - required: [contact_id]") {
		t.Fatalf("normalized schema retains unsupported SendCustomEventRequest selection anyOf:\n%s", got)
	}
	for _, wanted := range []string{
		"description: Exactly one effective identifier.",
		"event: { type: string, maxLength: 120 }",
		"contact_id:\n          anyOf:",
		"email:\n          anyOf:",
		"payload: { type: object, additionalProperties: true }",
	} {
		if !strings.Contains(got, wanted) {
			t.Errorf("normalized schema lost wire-relevant field or constraint %q:\n%s", wanted, got)
		}
	}
	if string(source) == got {
		t.Fatal("normalization did not change the unsupported selection union")
	}
	originalSchema := sendCustomEventRequestSchema(t, source)
	normalizedSchema := sendCustomEventRequestSchema(t, normalized)
	if _, ok := originalSchema["anyOf"]; !ok {
		t.Fatal("test fixture lost its documented identifier-selection union")
	}
	if _, ok := normalizedSchema["anyOf"]; ok {
		t.Fatal("normalized schema retains the unsupported identifier-selection union")
	}
	if !reflect.DeepEqual(originalSchema["properties"], normalizedSchema["properties"]) {
		t.Fatalf("normalization changed the request wire fields:\noriginal: %#v\nnormalized: %#v", originalSchema["properties"], normalizedSchema["properties"])
	}
	if !reflect.DeepEqual(originalSchema["required"], normalizedSchema["required"]) {
		t.Fatalf("normalization changed required wire fields:\noriginal: %#v\nnormalized: %#v", originalSchema["required"], normalizedSchema["required"])
	}
}

func sendCustomEventRequestSchema(t *testing.T, source []byte) map[string]any {
	t.Helper()
	asJSON, err := yaml.YAMLToJSON(source)
	if err != nil {
		t.Fatalf("YAMLToJSON() error = %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(asJSON, &document); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	components, ok := document["components"].(map[string]any)
	if !ok {
		t.Fatal("components is missing")
	}
	schemas, ok := components["schemas"].(map[string]any)
	if !ok {
		t.Fatal("components.schemas is missing")
	}
	schema, ok := schemas["SendCustomEventRequest"].(map[string]any)
	if !ok {
		t.Fatal("SendCustomEventRequest is missing")
	}
	return schema
}

func TestOgen_GeneratesSendCustomEventRequestAfterSelectionNormalization(t *testing.T) {
	temporary := t.TempDir()
	sourcePath := filepath.Join(temporary, "openapi.yaml")
	targetPath := filepath.Join(temporary, "api")
	configPath := filepath.Join(temporary, "ogen.yml")
	spec := `openapi: 3.1.1
info: { title: test, version: 1.0.0 }
paths:
  /v1/events/send:
    post:
      operationId: postEventsSend
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: '#/components/schemas/SendCustomEventRequest' }
      responses:
        '201': { description: Created }
components:
  schemas:
    UUID:
      type: string
      format: uuid
    SendCustomEventRequest:
      type: object
      additionalProperties: false
      required: [event]
      anyOf:
        - required: [contact_id]
          properties:
            contact_id: { $ref: '#/components/schemas/UUID' }
        - required: [email]
          properties:
            contact_id: { type: 'null' }
            email: { type: string, format: email }
      properties:
        event: { type: string }
        contact_id:
          anyOf:
            - { $ref: '#/components/schemas/UUID' }
            - { type: 'null' }
        email:
          anyOf:
            - { type: string }
            - { type: 'null' }
        payload: { type: object, additionalProperties: true }
`
	config := "generator:\n  features:\n    disable_all: true\n    enable:\n      - paths/client\n      - client/request/validation\n"
	normalized, err := normalizeCodegenSpec([]byte(spec))
	if err != nil {
		t.Fatalf("normalizeCodegenSpec() error = %v", err)
	}
	if err := os.WriteFile(sourcePath, normalized, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}

	command := exec.Command("go", "tool", "ogen", "--config", configPath, "--target", targetPath, "--package", "api", "--clean", sourcePath)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("ogen generation failed: %v\n%s", err, output)
	}
	generated, err := os.ReadFile(filepath.Join(targetPath, "oas_schemas_gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, wanted := range []string{"type SendCustomEventRequest struct", "ContactID", "Email", "Payload"} {
		if !strings.Contains(string(generated), wanted) {
			t.Errorf("generated request does not expose %q:\n%s", wanted, generated)
		}
	}
}

func TestNormalizeCodegenSpec_RequiresExactlyOneCanonicalUUIDSchema(t *testing.T) {
	for name, source := range map[string]string{
		"missing": `openapi: 3.1.1
components:
  schemas: {}
`,
		"duplicated": `openapi: 3.1.1
components:
  schemas:
    UUID:
      type: string
      format: uuid
other:
    UUID:
      type: string
      format: uuid
`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := normalizeCodegenSpec([]byte(source)); err == nil {
				t.Fatal("normalizeCodegenSpec() error = nil, want canonical UUID count error")
			}
		})
	}
}

func TestNormalizeCodegenSpec_TypesBareStringConstant(t *testing.T) {
	source := []byte(`openapi: 3.1.1
components:
  schemas:
    UUID:
      type: string
      format: uuid
    UntypedConstant:
      type: object
      properties:
        code:
          const: dynamic_segment_membership
`)

	normalized, err := normalizeCodegenSpec(source)
	if err != nil {
		t.Fatalf("normalizeCodegenSpec() error = %v", err)
	}
	if !strings.Contains(string(normalized), "code:\n          type: string\n          const: dynamic_segment_membership") {
		t.Fatalf("normalizeCodegenSpec() did not type bare string const:\n%s", normalized)
	}
}
