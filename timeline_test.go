package viapost

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ViaPost-io/viapost-go/api"
)

func TestRawMessageTimelineDecodesLegacyAndInboundPages(t *testing.T) {
	for _, test := range []struct {
		name    string
		include bool
		body    string
		field   string
		value   string
	}{
		{"legacy", false, `{"data":[{"id":"event-1","message_id":"message-1","type":"sent","occurred_at":"2026-10-02T12:00:00Z"}]}`, "message_id", "message-1"},
		{"inbound", true, `{"data":[{"id":"event-2","source":"inbound","inbound_message_id":"message-2","type":"inbound.received","occurred_at":"2026-10-02T12:00:00Z"}],"next_cursor":"cursor-v2"}`, "inbound_message_id", "message-2"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.URL.Query().Get("include"); (got == "inbound") != test.include {
					t.Errorf("include = %q, want inbound=%v", got, test.include)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			client, err := NewClient("vp_test_example", WithBaseURL(server.URL))
			if err != nil {
				t.Fatal(err)
			}
			params := api.GetMessagesEventsParams{}
			if test.include {
				params.Include = api.NewOptGetMessagesEventsInclude(api.GetMessagesEventsIncludeInbound)
			}
			response, err := client.Raw().GetMessagesEvents(context.Background(), params)
			if err != nil {
				t.Fatalf("GetMessagesEvents() error = %v", err)
			}
			page, ok := response.(*api.GetMessagesEventsOKHeaders)
			if !ok {
				t.Fatalf("GetMessagesEvents() response = %T", response)
			}
			if len(page.Response.Data) != 1 {
				t.Fatalf("data length = %d, want 1", len(page.Response.Data))
			}
			var value string
			if err := json.Unmarshal(page.Response.Data[0][test.field], &value); err != nil {
				t.Fatalf("decode %s: %v", test.field, err)
			}
			if value != test.value {
				t.Errorf("%s = %q, want %q", test.field, value, test.value)
			}
		})
	}
}
