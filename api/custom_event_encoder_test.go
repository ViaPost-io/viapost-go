package api

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestEncodePostEventsSendRequest_RequiresExactlyOneContactIdentifier(t *testing.T) {
	const contactID = "018f0000-0000-7000-8000-000000000001"

	tests := []struct {
		name     string
		request  SendCustomEventRequest
		wantErr  bool
		wantBody string
	}{
		{
			name: "contact ID only",
			request: SendCustomEventRequest{
				Event:     "purchase.completed",
				ContactID: NewOptNilUUID(UUID(uuid.MustParse(contactID))),
			},
			wantBody: `{"event":"purchase.completed","contact_id":"018f0000-0000-7000-8000-000000000001"}`,
		},
		{
			name: "email only",
			request: SendCustomEventRequest{
				Event: "purchase.completed",
				Email: NewOptNilString("person@example.com"),
			},
			wantBody: `{"event":"purchase.completed","email":"person@example.com"}`,
		},
		{
			name: "no identifier",
			request: SendCustomEventRequest{
				Event: "purchase.completed",
			},
			wantErr: true,
		},
		{
			name: "both identifiers",
			request: SendCustomEventRequest{
				Event:     "purchase.completed",
				ContactID: NewOptNilUUID(UUID(uuid.MustParse(contactID))),
				Email:     NewOptNilString("person@example.com"),
			},
			wantErr: true,
		},
		{
			name: "contact ID with empty email",
			request: SendCustomEventRequest{
				Event:     "purchase.completed",
				ContactID: NewOptNilUUID(UUID(uuid.MustParse(contactID))),
				Email:     NewOptNilString(""),
			},
			wantBody: `{"event":"purchase.completed","contact_id":"018f0000-0000-7000-8000-000000000001","email":""}`,
		},
		{
			name: "null contact ID with email",
			request: SendCustomEventRequest{
				Event:     "purchase.completed",
				ContactID: OptNilUUID{Set: true, Null: true},
				Email:     NewOptNilString("person@example.com"),
			},
			wantBody: `{"event":"purchase.completed","contact_id":null,"email":"person@example.com"}`,
		},
		{
			name: "null identifiers",
			request: SendCustomEventRequest{
				Event:     "purchase.completed",
				ContactID: OptNilUUID{Set: true, Null: true},
				Email:     OptNilString{Set: true, Null: true},
			},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest("POST", "/v1/events/send", nil)
			err := encodePostEventsSendRequest(&test.request, request)
			if test.wantErr {
				if err == nil {
					t.Fatal("encodePostEventsSendRequest() error = nil, want validation error")
				}
				return
			}
			if err != nil {
				t.Fatalf("encodePostEventsSendRequest() error = %v", err)
			}
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			if got := string(body); got != test.wantBody {
				t.Errorf("request body = %s, want %s", got, test.wantBody)
			}
			if contentType := request.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
				t.Errorf("Content-Type = %q, want application/json", contentType)
			}
		})
	}
}
