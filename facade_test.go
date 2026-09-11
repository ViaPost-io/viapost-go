package viapost

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResourceFacade_HTTPContracts(t *testing.T) {
	const (
		id        = "018f0000-0000-7000-8000-000000000001"
		timestamp = "2026-09-11T12:00:00Z"
	)
	message := `{"id":"` + id + `","status":"delivered","stream":"transactional","from_address":"a@example.com","to_address":"b@example.com","recipient_domain":"example.com","created_at":"` + timestamp + `"}`
	domain := `{"id":"` + id + `","name":"example.com","status":"verified","spf_verified":true,"dkim_verified":true,"dmarc_verified":true,"return_path_subdomain":"rp","created_at":"` + timestamp + `"}`
	template := `{"id":"` + id + `","name":"Welcome","created_at":"` + timestamp + `","updated_at":"` + timestamp + `"}`
	draft := `{"id":"` + id + `","version_number":1,"status":"draft","content_json":{},"variables":[],"created_at":"` + timestamp + `","updated_at":"` + timestamp + `"}`
	automation := `{"id":"` + id + `","name":"Welcome","status":"disabled","graph":{},"created_at":"` + timestamp + `","updated_at":"` + timestamp + `"}`

	tests := []struct {
		name       string
		method     string
		path       string
		statusCode int
		response   string
		call       func(context.Context, *Client) error
	}{
		{"messages get", http.MethodGet, "/v1/messages/" + id, http.StatusOK, message, func(ctx context.Context, client *Client) error { _, err := client.Messages.Get(ctx, id); return err }},
		{"messages events", http.MethodGet, "/v1/messages/" + id + "/events", http.StatusOK, `{"events":[]}`, func(ctx context.Context, client *Client) error { _, err := client.Messages.Events(ctx, id); return err }},
		{"messages metrics", http.MethodGet, "/v1/messages/metrics", http.StatusOK, `{"since":"` + timestamp + `","until":"` + timestamp + `","current":{"total":0,"delivered":0,"opened":0,"clicked":0,"bounced":0,"complained":0},"previous":{"total":0,"delivered":0,"opened":0,"clicked":0,"bounced":0,"complained":0},"timeseries":[],"by_domain":[]}`, func(ctx context.Context, client *Client) error {
			_, err := client.Messages.Metrics(ctx, 14, "")
			return err
		}},
		{"messages engagement", http.MethodGet, "/v1/messages/engagement", http.StatusOK, `{"since":"` + timestamp + `","delivered":0,"opened":0,"clicked":0}`, func(ctx context.Context, client *Client) error {
			_, err := client.Messages.Engagement(ctx, 14)
			return err
		}},
		{"messages timeseries", http.MethodGet, "/v1/messages/timeseries", http.StatusOK, `{"since":"` + timestamp + `","days":[]}`, func(ctx context.Context, client *Client) error {
			_, err := client.Messages.Timeseries(ctx, 14)
			return err
		}},
		{"domains list", http.MethodGet, "/v1/domains", http.StatusOK, `{"domains":[]}`, func(ctx context.Context, client *Client) error { _, err := client.Domains.List(ctx); return err }},
		{"domains get", http.MethodGet, "/v1/domains/" + id, http.StatusOK, domain, func(ctx context.Context, client *Client) error { _, err := client.Domains.Get(ctx, id); return err }},
		{"domains create", http.MethodPost, "/v1/domains", http.StatusCreated, `{"domain":` + domain + `,"dns_records":[]}`, func(ctx context.Context, client *Client) error {
			_, _, err := client.Domains.Create(ctx, "example.com")
			return err
		}},
		{"domains delete", http.MethodDelete, "/v1/domains/" + id, http.StatusNoContent, "", func(ctx context.Context, client *Client) error { return client.Domains.Delete(ctx, id) }},
		{"templates list", http.MethodGet, "/v1/templates", http.StatusOK, `{"templates":[]}`, func(ctx context.Context, client *Client) error {
			_, err := client.Templates.List(ctx, TemplateListOptions{})
			return err
		}},
		{"templates get", http.MethodGet, "/v1/templates/" + id, http.StatusOK, template, func(ctx context.Context, client *Client) error { _, err := client.Templates.Get(ctx, id); return err }},
		{"templates create", http.MethodPost, "/v1/templates", http.StatusCreated, `{"template":` + template + `,"draft":` + draft + `}`, func(ctx context.Context, client *Client) error {
			_, err := client.Templates.Create(ctx, "Welcome")
			return err
		}},
		{"templates delete", http.MethodDelete, "/v1/templates/" + id, http.StatusNoContent, "", func(ctx context.Context, client *Client) error { return client.Templates.Delete(ctx, id) }},
		{"webhooks list", http.MethodGet, "/v1/webhooks", http.StatusOK, `{"webhooks":[]}`, func(ctx context.Context, client *Client) error { _, err := client.Webhooks.List(ctx); return err }},
		{"webhooks delete", http.MethodDelete, "/v1/webhooks/" + id, http.StatusNoContent, "", func(ctx context.Context, client *Client) error { return client.Webhooks.Delete(ctx, id) }},
		{"automations list", http.MethodGet, "/v1/automations", http.StatusOK, `{"data":[]}`, func(ctx context.Context, client *Client) error {
			_, err := client.Automations.List(ctx, AutomationListOptions{})
			return err
		}},
		{"automations get", http.MethodGet, "/v1/automations/" + id, http.StatusOK, automation, func(ctx context.Context, client *Client) error { _, err := client.Automations.Get(ctx, id); return err }},
		{"automations create", http.MethodPost, "/v1/automations", http.StatusCreated, automation, func(ctx context.Context, client *Client) error {
			_, err := client.Automations.Create(ctx, "Welcome")
			return err
		}},
		{"automations delete", http.MethodDelete, "/v1/automations/" + id, http.StatusNoContent, "", func(ctx context.Context, client *Client) error { return client.Automations.Delete(ctx, id) }},
		{"usage get", http.MethodGet, "/v1/usage", http.StatusOK, `{"period":{"start":"` + timestamp + `","end":"` + timestamp + `","timezone":"UTC"},"used":1,"limit":100,"remaining":99,"unlimited":false}`, func(ctx context.Context, client *Client) error { _, err := client.Usage.Get(ctx); return err }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				if got := request.Method; got != test.method {
					t.Errorf("method = %q, want %q", got, test.method)
				}
				if got := request.URL.Path; got != test.path {
					t.Errorf("path = %q, want %q", got, test.path)
				}
				if request.Header.Get("Authorization") != "Bearer vp_test_example" {
					t.Error("missing Bearer authorization")
				}
				if test.response != "" {
					w.Header().Set("Content-Type", "application/json")
				}
				w.WriteHeader(test.statusCode)
				_, _ = w.Write([]byte(test.response))
			}))
			defer server.Close()

			client, err := NewClient("vp_test_example", WithBaseURL(server.URL))
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}
			if err := test.call(context.Background(), client); err != nil {
				t.Fatalf("facade call error = %v", err)
			}
		})
	}
}
