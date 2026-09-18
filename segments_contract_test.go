package viapost

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ViaPost-io/viapost-go/api"
	"github.com/go-faster/jx"
)

func TestGeneratedClient_SegmentContactsReturnsPaginatedList(t *testing.T) {
	const (
		segmentID = "018f0000-0000-7000-8000-000000000001"
		contactID = "018f0000-0000-7000-8000-000000000002"
	)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/v1/segments/"+segmentID+"/contacts" {
			t.Errorf("request = %s %s, want GET /v1/segments/%s/contacts", request.Method, request.URL.Path, segmentID)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"data":[{"id":"` + contactID + `","email":"person@example.com","subscribed":true,"properties":{},"created_at":"2026-09-16T12:00:00Z","updated_at":"2026-09-16T12:00:00Z"}],"next_cursor":"next-page"}`))
	}))
	defer server.Close()

	client, err := NewClient("vp_test_example", WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	result, err := client.Raw().GetSegmentsIDContacts(context.Background(), api.GetSegmentsIDContactsParams{ID: segmentID})
	if err != nil {
		t.Fatalf("GetSegmentsIDContacts() error = %v", err)
	}
	list, ok := result.(*api.ContactList)
	if !ok {
		t.Fatalf("GetSegmentsIDContacts() response = %T, want *api.ContactList", result)
	}
	if len(list.Data) != 1 || list.Data[0].Email != "person@example.com" {
		t.Fatalf("contacts = %#v, want one person@example.com contact", list.Data)
	}
	if list.NextCursor != "next-page" {
		t.Fatalf("next cursor = %q, want next-page", list.NextCursor)
	}
}

func TestGeneratedClient_SegmentVariantsRoundTrip(t *testing.T) {
	const definition = `{"all":[{"field":"email","operator":"contains","value":"@example.com"}]}`
	staticSegment := `{"id":"018f0000-0000-7000-8000-000000000001","name":"static","description":null,"kind":"static","definition":null,"contact_count":2,"created_at":"2026-09-16T12:00:00Z","updated_at":"2026-09-16T12:00:00Z"}`
	dynamicSegment := `{"id":"018f0000-0000-7000-8000-000000000002","name":"dynamic","description":"rule","kind":"dynamic","definition":` + definition + `,"contact_count":3,"created_at":"2026-09-16T12:00:00Z","updated_at":"2026-09-16T12:00:00Z"}`
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		switch request.Method + " " + request.URL.Path {
		case "POST /v1/segments":
			response.WriteHeader(http.StatusCreated)
			var body map[string]json.RawMessage
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Errorf("decode create request: %v", err)
				return
			}
			var kind string
			if raw, ok := body["kind"]; ok {
				_ = json.Unmarshal(raw, &kind)
			}
			if kind == "dynamic" {
				if string(body["definition"]) != definition {
					t.Errorf("dynamic definition = %s, want %s", body["definition"], definition)
				}
				_, _ = response.Write([]byte(dynamicSegment))
				return
			}
			if _, ok := body["definition"]; ok {
				t.Errorf("static request unexpectedly sent definition")
			}
			_, _ = response.Write([]byte(staticSegment))
		case "GET /v1/segments":
			_, _ = response.Write([]byte(`{"data":[` + staticSegment + `,` + dynamicSegment + `],"next_cursor":""}`))
		case "POST /v1/segments/preview":
			var body struct {
				Definition json.RawMessage `json:"definition"`
			}
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Errorf("decode preview request: %v", err)
				return
			}
			if string(body.Definition) != definition {
				t.Errorf("preview definition = %s, want %s", body.Definition, definition)
			}
			_, _ = response.Write([]byte(`{"contact_count":3,"data":[]}`))
		case "PATCH /v1/segments/018f0000-0000-7000-8000-000000000002":
			var body struct {
				Definition json.RawMessage `json:"definition"`
			}
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Errorf("decode patch request: %v", err)
				return
			}
			if string(body.Definition) != definition {
				t.Errorf("patch definition = %s, want %s", body.Definition, definition)
			}
			_, _ = response.Write([]byte(dynamicSegment))
		default:
			t.Errorf("unexpected request %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient("vp_test_example", WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	definitionValue := api.SegmentDefinition{"all": jx.Raw(`[{"field":"email","operator":"contains","value":"@example.com"}]`)}
	staticRequest := api.CreateSegmentRequest{"name": rawJSON(t, "static"), "description": rawJSON(t, "plain")}
	dynamicRequest := api.CreateSegmentRequest{"name": rawJSON(t, "dynamic"), "kind": rawJSON(t, "dynamic"), "definition": rawJSON(t, json.RawMessage(definition))}
	for _, request := range []api.CreateSegmentRequest{staticRequest, dynamicRequest} {
		result, err := client.Raw().PostSegments(context.Background(), request)
		if err != nil {
			t.Fatalf("PostSegments() error = %v", err)
		}
		segment, ok := result.(*api.Segment)
		if !ok {
			t.Fatalf("PostSegments() result = %T, want *api.Segment", result)
		}
		if err := segment.Validate(); err != nil {
			t.Fatalf("returned segment validation: %v", err)
		}
	}
	listResult, err := client.Raw().GetSegments(context.Background(), api.GetSegmentsParams{})
	if err != nil {
		t.Fatalf("GetSegments() error = %v", err)
	}
	list, ok := listResult.(*api.SegmentList)
	if !ok || len(list.Data) != 2 {
		t.Fatalf("GetSegments() result = %#v, want two segment variants", listResult)
	}
	previewResult, err := client.Raw().PostSegmentsPreview(context.Background(), &api.SegmentPreviewRequest{Definition: definitionValue})
	if err != nil {
		t.Fatalf("PostSegmentsPreview() error = %v", err)
	}
	if _, ok := previewResult.(*api.SegmentPreview); !ok {
		t.Fatalf("PostSegmentsPreview() result = %T, want *api.SegmentPreview", previewResult)
	}
	patched := &api.UpdateSegmentRequest{Definition: api.NewOptSegmentDefinition(definitionValue)}
	patchResult, err := client.Raw().PatchSegmentsID(context.Background(), patched, api.PatchSegmentsIDParams{ID: "018f0000-0000-7000-8000-000000000002"})
	if err != nil {
		t.Fatalf("PatchSegmentsID() error = %v", err)
	}
	if _, ok := patchResult.(*api.Segment); !ok {
		t.Fatalf("PatchSegmentsID() result = %T, want *api.Segment", patchResult)
	}
}

func TestGeneratedClient_RejectsInvalidDynamicSegmentUnion(t *testing.T) {
	request := api.CreateSegmentRequest{
		"name": rawJSON(t, "dynamic"),
		"kind": rawJSON(t, "dynamic"),
	}
	if err := request.Validate(); err == nil {
		t.Fatal("dynamic segment request without definition was accepted")
	}
	definition := api.SegmentDefinition{"all": jx.Raw(`[]`)}
	if err := definition.Validate(); err == nil {
		t.Fatal("empty segment definition group was accepted")
	}
}

func TestGeneratedClient_SegmentDefinitionContractVariantsAndBounds(t *testing.T) {
	valid := []string{
		`{"all":[{"field":"email","operator":"eq","value":"a@example.com"}]}`,
		`{"all":[{"field":"first_name","operator":"is_set"}]}`,
		`{"all":[{"field":"subscribed","operator":"eq","value":true}]}`,
		`{"all":[{"field":"created_at","operator":"after","value":"2026-09-16T12:00:00Z"}]}`,
		`{"all":[{"field":"property","key":"plan","operator":"eq","value":"pro"}]}`,
		`{"all":[{"field":"property","key":"plan","operator":"exists"}]}`,
		`{"all":[{"event_name":"purchase.completed","operator":"occurred","within_days":30}]}`,
		`{"all":[{"any":[{"all":[{"any":[{"field":"email","operator":"eq","value":"a@example.com"}]}]}]}]}`,
	}
	for _, input := range valid {
		t.Run(input, func(t *testing.T) {
			var definition api.SegmentDefinition
			if err := json.Unmarshal([]byte(input), &definition); err != nil {
				t.Fatalf("unmarshal definition: %v", err)
			}
			if err := definition.Validate(); err != nil {
				t.Fatalf("valid definition rejected: %v", err)
			}
		})
	}
	invalid := []string{
		`{"all":[{"field":"email","operator":"eq"}]}`,
		`{"all":[{"field":"email","operator":"eq","value":null}]}`,
		`{"all":[{"field":"subscribed","operator":"eq","value":null}]}`,
		`{"all":[{"field":"property","key":"","operator":"exists"}]}`,
		`{"all":[{"event_name":"viapost:internal","operator":"occurred","within_days":1}]}`,
		`{"all":[{"all":[{"any":[{"all":[{"any":[{"field":"email","operator":"eq","value":"a@example.com"}]}]}]}]}]}`,
	}
	for _, input := range invalid {
		var definition api.SegmentDefinition
		if err := json.Unmarshal([]byte(input), &definition); err != nil {
			t.Fatalf("unmarshal definition: %v", err)
		}
		if err := definition.Validate(); err == nil {
			t.Fatalf("invalid definition accepted: %s", input)
		}
	}
	var tooMany api.SegmentDefinition
	children := make([]map[string]any, 5)
	for i := range children {
		leaves := make([]map[string]any, 21)
		for j := range leaves {
			leaves[j] = map[string]any{"field": "email", "operator": "eq", "value": "a@example.com"}
		}
		children[i] = map[string]any{"all": leaves}
	}
	encoded, err := json.Marshal(map[string]any{"all": children})
	if err != nil {
		t.Fatalf("marshal max predicate fixture: %v", err)
	}
	if err := json.Unmarshal(encoded, &tooMany); err != nil {
		t.Fatalf("unmarshal max predicate fixture: %v", err)
	}
	if err := tooMany.Validate(); err == nil {
		t.Fatal("definition with 101 predicates was accepted")
	}
}

func rawJSON(t *testing.T, value any) jx.Raw {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal(%#v): %v", value, err)
	}
	return jx.Raw(encoded)
}
