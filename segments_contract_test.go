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
	const definition = `{"all":[{"text_attribute":{"field":"email","operator":"contains","value":"@example.com"}}]}`
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
		default:
			t.Errorf("unexpected request %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient("vp_test_example", WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	definitionValue := api.SegmentDefinition{"all": jx.Raw(`[{"text_attribute":{"field":"email","operator":"contains","value":"@example.com"}}]`)}
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

func rawJSON(t *testing.T, value any) jx.Raw {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal(%#v): %v", value, err)
	}
	return jx.Raw(encoded)
}
