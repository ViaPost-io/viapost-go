package generate

import (
	"strings"
	"testing"
)

func TestNormalizeCodegenSpec_PreservesSourceAndRemovesUnsupportedConstraints(t *testing.T) {
	source := []byte(`openapi: 3.1.1
schema:
  id: { type: string, format: uuid }
  status: { type: string, enum: [ready, failed] }
  items: false
components:
  schemas:
    UUID: { type: string, format: uuid }
security:
  - bearerApiKey: []
  - sessionCookie: []
parameters: [{ $ref: '#/components/parameters/CsrfHeader' }]
`)

	got := string(normalizeCodegenSpec(source))

	for _, unwanted := range []string{"id: { type: string, format: uuid }", "items: false", "sessionCookie", "CsrfHeader"} {
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
	if !strings.Contains(string(source), "items: false") {
		t.Fatal("normalization mutated the source buffer")
	}
}
