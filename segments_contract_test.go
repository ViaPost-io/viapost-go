package viapost

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ViaPost-io/viapost-go/api"
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
	if cursor, ok := list.NextCursor.Get(); !ok || cursor != "next-page" {
		t.Fatalf("next cursor = %q, %t, want next-page, true", cursor, ok)
	}
}
