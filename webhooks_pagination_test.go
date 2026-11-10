package viapost

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWebhooksListRetrievesAllPages(t *testing.T) {
	const endpoint = `{"name":"test webhook","payload_version":"2026-10","last_delivery":null,"id":"018f0000-0000-7000-8000-000000000001","url":"https://hooks.example.com/viapost","event_types":["delivered"],"enabled":true,"max_attempts":5,"consecutive_failures":0,"disabled_at":null,"secret_rotated_at":null,"version":1,"created_at":"2026-09-11T12:00:00Z","updated_at":"2026-09-11T12:00:00Z"}`
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		if requests == 1 {
			if got := r.URL.Query().Get("cursor"); got != "" {
				t.Fatalf("first page cursor = %q, want empty", got)
			}
			_, _ = w.Write([]byte(`{"webhooks":[` + endpoint + `],"next_cursor":"page-2"}`))
			return
		}
		if got := r.URL.Query().Get("cursor"); got != "page-2" {
			t.Errorf("second page cursor = %q, want page-2", got)
		}
		_, _ = w.Write([]byte(`{"webhooks":[` + endpoint + `]}`))
	}))
	defer server.Close()

	client, err := NewClient("vp_test_example", WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	items, err := client.Webhooks.List(context.Background())
	if err != nil {
		t.Fatalf("Webhooks.List() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("Webhooks.List() returned %d items, want 2", len(items))
	}
	if requests != 2 {
		t.Fatalf("server received %d requests, want 2", requests)
	}
}

func TestWebhooksListRejectsRepeatedCursor(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"webhooks":[],"next_cursor":"same"}`))
	}))
	defer server.Close()

	client, err := NewClient("vp_test_example", WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if _, err := client.Webhooks.List(context.Background()); err == nil {
		t.Fatal("Webhooks.List() error = nil, want repeated cursor error")
	}
	if requests != 2 {
		t.Fatalf("server received %d requests, want 2", requests)
	}
}
