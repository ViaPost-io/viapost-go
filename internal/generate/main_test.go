package generate

import (
	"strings"
	"testing"
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

func TestRelaxSegmentObjectUnions(t *testing.T) {
	source := `    Segment:
      oneOf:
        - $ref: '#/components/schemas/StaticSegment'
        - $ref: '#/components/schemas/DynamicSegment'
    StaticSegment:
    CreateSegmentRequest:
      oneOf:
        - type: object
    UpdateSegmentRequest:
    SegmentDefinition:
      description: bounded tree
      oneOf:
        - type: object
    AllGroupDepth1:
    SegmentRuleDepth1:
      oneOf:
        - type: object
    SegmentRuleDepth2:
    SegmentRuleDepth2:
      oneOf:
        - type: object
    SegmentRuleDepth3:
    SegmentRuleDepth3:
      oneOf:
        - type: object
    LeafRule:
    LeafRule:
      oneOf:
        - type: object
    TextAttributePredicate:
`

	got, err := relaxSegmentObjectUnions(source)
	if err != nil {
		t.Fatalf("relaxSegmentObjectUnions() error = %v", err)
	}
	for _, name := range []string{"Segment", "CreateSegmentRequest", "SegmentDefinition", "SegmentRuleDepth1", "SegmentRuleDepth2", "SegmentRuleDepth3", "LeafRule"} {
		if !strings.Contains(got, "    "+name+":\n      type: object") {
			t.Fatalf("normalized schema did not relax %s:\n%s", name, got)
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
