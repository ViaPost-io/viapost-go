package viapost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ViaPost-io/viapost-go/api"
)

func TestEmailSend_SendsAuthenticatedRequestAndDecodesResult(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Method, http.MethodPost; got != want {
			t.Errorf("method = %q, want %q", got, want)
		}
		if got, want := r.URL.Path, "/v1/send"; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		if got, want := r.Header.Get("Authorization"), "Bearer vp_test_example"; got != want {
			t.Errorf("Authorization = %q, want %q", got, want)
		}
		if got, want := r.Header.Get("User-Agent"), "viapost-test/1.0"; got != want {
			t.Errorf("User-Agent = %q, want %q", got, want)
		}
		if got, want := r.Header.Get("Idempotency-Key"), "send-123"; got != want {
			t.Errorf("Idempotency-Key = %q, want %q", got, want)
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if got, want := body["from"], "hello@example.com"; got != want {
			t.Errorf("from = %v, want %v", got, want)
		}
		if got, want := body["metadata"].(map[string]any)["order_id"], "order-42"; got != want {
			t.Errorf("metadata.order_id = %v, want %v", got, want)
		}
		if got, want := body["variables"].(map[string]any)["name"], "Ada"; got != want {
			t.Errorf("variables.name = %v, want %v", got, want)
		}
		attachments := body["attachments"].([]any)
		if got, want := attachments[0].(map[string]any)["filename"], "hello.txt"; got != want {
			t.Errorf("attachments[0].filename = %v, want %v", got, want)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"accepted":[{"message_id":"018f0000-0000-7000-8000-000000000001","to":"user@example.com"}],"rejected":null}`))
	}))
	defer server.Close()

	client, err := NewClient(
		"vp_test_example",
		WithBaseURL(server.URL),
		WithUserAgent("viapost-test/1.0"),
	)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	result, err := client.Email.Send(context.Background(), SendRequest{
		From:      "hello@example.com",
		To:        []string{"user@example.com"},
		Subject:   "SDK test",
		Text:      "hello",
		Metadata:  map[string]any{"order_id": "order-42"},
		Variables: map[string]any{"name": "Ada"},
		Attachments: []Attachment{{
			Filename:    "hello.txt",
			Content:     []byte("hello"),
			ContentType: "text/plain",
		}},
	}, WithIdempotencyKey("send-123"))
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if got, want := len(result.Accepted), 1; got != want {
		t.Fatalf("accepted length = %d, want %d", got, want)
	}
	if got, want := result.Accepted[0].To, "user@example.com"; got != want {
		t.Errorf("accepted recipient = %q, want %q", got, want)
	}
}

func TestEmailSend_RespectsConfiguredTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(150 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"accepted":null,"rejected":null}`))
	}))
	defer server.Close()

	client, err := NewClient(
		"vp_test_example",
		WithBaseURL(server.URL),
		WithTimeout(20*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	started := time.Now()
	_, err = client.Email.Send(context.Background(), SendRequest{From: "a@example.com", To: []string{"b@example.com"}})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Send() error = %v, want context deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed >= 100*time.Millisecond {
		t.Fatalf("Send() elapsed = %s, configured timeout was not respected", elapsed)
	}
}

func TestNewClient_RejectsNonHTTPBaseURL(t *testing.T) {
	_, err := NewClient("vp_test_example", WithBaseURL("ftp://api.example.com"))
	if !errors.Is(err, ErrInvalidBaseURL) {
		t.Fatalf("NewClient() error = %v, want ErrInvalidBaseURL", err)
	}
}

func TestNewClient_RejectsPlainHTTPOutsideLoopback(t *testing.T) {
	_, err := NewClient("vp_test_example", WithBaseURL("http://api.example.com"))
	if !errors.Is(err, ErrInsecureBaseURL) {
		t.Fatalf("NewClient() error = %v, want ErrInsecureBaseURL", err)
	}
}

func TestNewClient_RejectsNonPositiveTimeout(t *testing.T) {
	_, err := NewClient("vp_test_example", WithTimeout(0))
	if !errors.Is(err, ErrInvalidTimeout) {
		t.Fatalf("NewClient() error = %v, want ErrInvalidTimeout", err)
	}
}

func TestMessagesList_EncodesPaginationAndFilters(t *testing.T) {
	const messageID = "018f0000-0000-7000-8000-000000000001"
	cursor := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		for name, want := range map[string]string{
			"cursor":     cursor.Format(time.RFC3339),
			"limit":      "25",
			"status":     "delivered",
			"search":     "order",
			"period":     "7d",
			"api_key_id": "key-id",
		} {
			if got := query.Get(name); got != want {
				t.Errorf("query %s = %q, want %q", name, got, want)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"messages":[{"id":"` + messageID + `","status":"delivered","stream":"transactional","from_address":"a@example.com","to_address":"b@example.com","recipient_domain":"example.com","created_at":"2026-09-11T12:00:00Z"}]}`))
	}))
	defer server.Close()

	client, err := NewClient("vp_test_example", WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	messages, err := client.Messages.List(context.Background(), MessageListOptions{
		Cursor:   &cursor,
		Limit:    25,
		Status:   "delivered",
		Search:   "order",
		Period:   "7d",
		APIKeyID: "key-id",
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if got, want := len(messages), 1; got != want {
		t.Fatalf("messages length = %d, want %d", got, want)
	}
	if got, want := string(messages[0].ID), messageID; got != want {
		t.Errorf("message ID = %q, want %q", got, want)
	}
}

func TestListFilters_ValidateEnumsBeforeNetwork(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/v1/messages":
			_, _ = w.Write([]byte(`{"messages":[]}`))
		case "/v1/automations":
			_, _ = w.Write([]byte(`{"data":[]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client, err := NewClient("vp_test_example", WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	for _, period := range []MessagePeriod{"", MessagePeriod24Hours, MessagePeriod7Days, MessagePeriod14Days, MessagePeriod30Days} {
		if _, err := client.Messages.List(context.Background(), MessageListOptions{Period: period}); err != nil {
			t.Fatalf("Messages.List() period %q error = %v", period, err)
		}
	}
	for _, status := range []AutomationStatus{"", AutomationStatusDisabled, AutomationStatusEnabled, AutomationStatusArchived} {
		if _, err := client.Automations.List(context.Background(), AutomationListOptions{Status: status}); err != nil {
			t.Fatalf("Automations.List() status %q error = %v", status, err)
		}
	}
	if got, want := requests.Load(), int32(9); got != want {
		t.Fatalf("request count after valid enum filters = %d, want %d", got, want)
	}

	if _, err := client.Messages.List(context.Background(), MessageListOptions{Period: MessagePeriod("90d")}); !errors.Is(err, ErrInvalidMessagePeriod) {
		t.Fatalf("Messages.List() invalid period error = %v, want ErrInvalidMessagePeriod", err)
	}
	if _, err := client.Automations.List(context.Background(), AutomationListOptions{Status: AutomationStatus("paused")}); !errors.Is(err, ErrInvalidAutomationStatus) {
		t.Fatalf("Automations.List() invalid status error = %v, want ErrInvalidAutomationStatus", err)
	}
	if got, want := requests.Load(), int32(9); got != want {
		t.Fatalf("request count after invalid enum filters = %d, want %d", got, want)
	}
}

func TestNewClient_InitializesResourceServices(t *testing.T) {
	client, err := NewClient("vp_test_example")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	for name, service := range map[string]any{
		"automations": client.Automations,
		"domains":     client.Domains,
		"templates":   client.Templates,
		"usage":       client.Usage,
		"webhooks":    client.Webhooks,
	} {
		if service == nil {
			t.Errorf("%s service is nil", name)
		}
	}
}

func TestEmailSend_RespectsContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"accepted":null,"rejected":null}`))
	}))
	defer server.Close()

	client, err := NewClient("vp_test_example", WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = client.Email.Send(ctx, SendRequest{From: "a@example.com", To: []string{"b@example.com"}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Send() error = %v, want context canceled", err)
	}
}

func TestEmailSend_ReturnsTypedAPIErrorWithRequestID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-ID", "req_header")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"code":"quota_exceeded","message":"Monthly limit reached","request_id":"req_body"}}`))
	}))
	defer server.Close()

	client, err := NewClient("vp_test_example", WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	_, err = client.Email.Send(context.Background(), SendRequest{From: "a@example.com", To: []string{"b@example.com"}})
	if err == nil {
		t.Fatal("Send() error = nil, want APIError")
	}

	var apiError *APIError
	if !errors.As(err, &apiError) {
		t.Fatalf("Send() error type = %T, want *APIError", err)
	}
	if got, want := apiError.StatusCode, http.StatusTooManyRequests; got != want {
		t.Errorf("StatusCode = %d, want %d", got, want)
	}
	if got, want := apiError.Code, "quota_exceeded"; got != want {
		t.Errorf("Code = %q, want %q", got, want)
	}
	if got, want := apiError.Message, "Monthly limit reached"; got != want {
		t.Errorf("Message = %q, want %q", got, want)
	}
	if got, want := apiError.RequestID, "req_header"; got != want {
		t.Errorf("RequestID = %q, want %q", got, want)
	}
	if len(apiError.Body) == 0 {
		t.Error("Body is empty")
	}
}

func TestEmailSend_DoesNotRetryMutations(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"code":"internal_error","message":"try again later"}}`))
	}))
	defer server.Close()

	client, err := NewClient("vp_test_example", WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	_, err = client.Email.Send(context.Background(), SendRequest{From: "a@example.com", To: []string{"b@example.com"}})
	if err == nil {
		t.Fatal("Send() error = nil, want non-nil")
	}
	if got, want := requests.Load(), int32(1); got != want {
		t.Fatalf("request count = %d, want %d; mutations must not retry automatically", got, want)
	}
}

func TestEmailSend_ValidatesVariablesLimitBeforeNetwork(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"accepted":null,"rejected":null}`))
	}))
	defer server.Close()

	client, err := NewClient("vp_test_example", WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	baseRequest := SendRequest{From: "a@example.com", To: []string{"b@example.com"}}

	request := baseRequest
	request.Variables = makeTestObject(100)
	if _, err := client.Email.Send(context.Background(), request); err != nil {
		t.Fatalf("Send() with 100 variables error = %v", err)
	}
	if got, want := requests.Load(), int32(1); got != want {
		t.Fatalf("request count after valid request = %d, want %d", got, want)
	}

	request = baseRequest
	request.Variables = makeTestObject(101)
	_, err = client.Email.Send(context.Background(), request)
	if !errors.Is(err, ErrTooManyVariables) {
		t.Fatalf("Send() with 101 variables error = %v, want ErrTooManyVariables", err)
	}
	if got, want := requests.Load(), int32(1); got != want {
		t.Fatalf("request count after invalid request = %d, want %d", got, want)
	}
}

func TestEmailSend_ValidatesStreamBeforeNetwork(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"accepted":null,"rejected":null}`))
	}))
	defer server.Close()

	client, err := NewClient("vp_test_example", WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	for _, stream := range []Stream{"", StreamTransactional, StreamMarketing} {
		_, err := client.Email.Send(context.Background(), SendRequest{
			From: "a@example.com", To: []string{"b@example.com"}, Stream: stream,
		})
		if err != nil {
			t.Fatalf("Send() stream %q error = %v", stream, err)
		}
	}
	_, err = client.Email.Send(context.Background(), SendRequest{
		From: "a@example.com", To: []string{"b@example.com"}, Stream: Stream("future"),
	})
	if !errors.Is(err, ErrInvalidStream) {
		t.Fatalf("Send() invalid stream error = %v, want ErrInvalidStream", err)
	}
	if got, want := requests.Load(), int32(3); got != want {
		t.Fatalf("request count = %d, want %d", got, want)
	}
}

func TestEmailSend_ValidatesAndEncodesTemplateUUIDBeforeNetwork(t *testing.T) {
	const templateID = "018f0000-0000-7000-8000-000000000123"
	validTemplateID := templateID
	invalidTemplateID := "not-a-uuid"
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		var body struct {
			TemplateID string `json:"template_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body.TemplateID != templateID {
			t.Errorf("template_id = %q, want %q", body.TemplateID, templateID)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"accepted":null,"rejected":null}`))
	}))
	defer server.Close()

	client, err := NewClient("vp_test_example", WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if _, err := client.Email.Send(context.Background(), SendRequest{
		From: "a@example.com", To: []string{"b@example.com"}, TemplateID: &validTemplateID,
	}); err != nil {
		t.Fatalf("Send() valid template UUID error = %v", err)
	}
	if got, want := requests.Load(), int32(1); got != want {
		t.Fatalf("request count after valid template UUID = %d, want %d", got, want)
	}

	if _, err := client.Email.Send(context.Background(), SendRequest{
		From: "a@example.com", To: []string{"b@example.com"}, TemplateID: &invalidTemplateID,
	}); !errors.Is(err, ErrInvalidTemplateID) {
		t.Fatalf("Send() invalid template UUID error = %v, want ErrInvalidTemplateID", err)
	}
	if got, want := requests.Load(), int32(1); got != want {
		t.Fatalf("request count after invalid template UUID = %d, want %d", got, want)
	}
}

func TestClient_ThreeHundredResponseIsTypedAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "/elsewhere")
		w.Header().Set("X-Request-ID", "req-redirect")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer server.Close()

	client, err := NewClient("vp_test_example", WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	_, err = client.Usage.Get(context.Background())
	var apiError *APIError
	if !errors.As(err, &apiError) {
		t.Fatalf("Usage.Get() error = %T %v, want *APIError", err, err)
	}
	if apiError.StatusCode != http.StatusTemporaryRedirect || apiError.RequestID != "req-redirect" {
		t.Fatalf("APIError = %#v", apiError)
	}
}

func TestSDKTransport_OneHundredResponseIsTypedAPIError(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "https://api.example.com/v1/usage", nil)
	transport := &sdkTransport{
		origin: "https://api.example.com:443",
		next: roundTripperFunc(func(got *http.Request) (*http.Response, error) {
			header := make(http.Header)
			header.Set("X-Request-ID", "req-informational")
			return &http.Response{
				StatusCode: http.StatusSwitchingProtocols,
				Header:     header,
				Body:       http.NoBody,
				Request:    got,
			}, nil
		}),
	}

	_, err := transport.RoundTrip(request)
	var apiError *APIError
	if !errors.As(err, &apiError) {
		t.Fatalf("RoundTrip() error = %T %v, want *APIError", err, err)
	}
	if apiError.StatusCode != http.StatusSwitchingProtocols || apiError.RequestID != "req-informational" {
		t.Fatalf("APIError = %#v", apiError)
	}
}

func TestClient_RawServerOverrideCannotExfiltrateBearerCrossOrigin(t *testing.T) {
	base := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer base.Close()

	var evilRequests atomic.Int32
	var evilAuthorization atomic.Value
	evil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		evilRequests.Add(1)
		evilAuthorization.Store(r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"used":0,"limit":1,"remaining":1,"unlimited":false}`))
	}))
	defer evil.Close()

	client, err := NewClient("vp_test_secret", WithBaseURL(base.URL))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	evilURL, err := url.Parse(evil.URL)
	if err != nil {
		t.Fatalf("parse evil URL: %v", err)
	}
	_, err = client.Raw().GetUsage(api.WithServerURL(context.Background(), evilURL))
	if !errors.Is(err, ErrCrossOriginRequest) {
		t.Fatalf("Raw().GetUsage() error = %v, want ErrCrossOriginRequest", err)
	}
	if got := evilRequests.Load(); got != 0 {
		t.Fatalf("evil request count = %d, want 0", got)
	}
	if got, loaded := evilAuthorization.Load().(string); loaded && got != "" {
		t.Fatalf("evil Authorization = %q, want empty", got)
	}
}

func TestWebhooksCreate_ValidatesURLBeforeNetwork(t *testing.T) {
	t.Run("absolute HTTPS webhook URLs are accepted over HTTP and HTTPS API transports", func(t *testing.T) {
		for _, useTLS := range []bool{false, true} {
			t.Run(map[bool]string{false: "http", true: "https"}[useTLS], func(t *testing.T) {
				var requests atomic.Int32
				handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					requests.Add(1)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					_, _ = w.Write([]byte(`{"endpoint":{"id":"018f0000-0000-7000-8000-000000000001","url":"https://hooks.example.com/viapost","event_types":["delivered"],"enabled":true,"max_attempts":5,"consecutive_failures":0,"disabled_at":null,"secret_rotated_at":null,"version":1,"created_at":"2026-09-11T12:00:00Z","updated_at":"2026-09-11T12:00:00Z"},"secret":"0123456789012345678901234567890123456789012"}`))
				})

				var server *httptest.Server
				if useTLS {
					server = httptest.NewTLSServer(handler)
				} else {
					server = httptest.NewServer(handler)
				}
				defer server.Close()

				options := []Option{WithBaseURL(server.URL)}
				if useTLS {
					options = append(options, WithHTTPClient(server.Client()))
				}
				client, err := NewClient("vp_test_example", options...)
				if err != nil {
					t.Fatalf("NewClient() error = %v", err)
				}
				webhookURL := "https://hooks.example.com/viapost"
				if _, err := client.Webhooks.Create(context.Background(), webhookURL, []string{"delivered"}); err != nil {
					t.Fatalf("Create() error = %v", err)
				}
				if got, want := requests.Load(), int32(1); got != want {
					t.Fatalf("request count = %d, want %d", got, want)
				}
			})
		}
	})

	t.Run("non-HTTPS, relative, credentialed, fragmented, and hostless URLs are rejected", func(t *testing.T) {
		var requests atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			requests.Add(1)
			w.WriteHeader(http.StatusCreated)
		}))
		defer server.Close()

		client, err := NewClient("vp_test_example", WithBaseURL(server.URL))
		if err != nil {
			t.Fatalf("NewClient() error = %v", err)
		}
		for _, endpoint := range []string{
			"/relative",
			"http://hooks.example.com/viapost",
			"ftp://hooks.example.com/viapost",
			"javascript:alert(1)",
			"https://user:password@hooks.example.com/viapost",
			"https://hooks.example.com/viapost#secret",
			"https://:443/viapost",
		} {
			_, err := client.Webhooks.Create(context.Background(), endpoint, []string{"delivered"})
			if !errors.Is(err, ErrInvalidWebhookURL) {
				t.Errorf("Create(%q) error = %v, want ErrInvalidWebhookURL", endpoint, err)
			}
		}
		if got := requests.Load(); got != 0 {
			t.Fatalf("request count = %d, want 0", got)
		}
	})
}

func TestEmailSend_ValidatesAttachmentAndIdempotencyBoundariesBeforeNetwork(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"accepted":null,"rejected":null}`))
	}))
	defer server.Close()

	client, err := NewClient("vp_test_example", WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	baseRequest := SendRequest{From: "a@example.com", To: []string{"b@example.com"}}

	validRequest := baseRequest
	validRequest.Attachments = makeTestAttachments(MaxAttachments)
	if _, err := client.Email.Send(context.Background(), validRequest, WithIdempotencyKey(strings.Repeat("k", MaxIdempotencyKeyLength))); err != nil {
		t.Fatalf("Send() at attachment/idempotency limits error = %v", err)
	}
	if got, want := requests.Load(), int32(1); got != want {
		t.Fatalf("request count after valid request = %d, want %d", got, want)
	}

	tests := []struct {
		name    string
		request SendRequest
		options []SendOption
		wantErr error
	}{
		{
			name:    "too many attachments",
			request: SendRequest{From: baseRequest.From, To: baseRequest.To, Attachments: makeTestAttachments(MaxAttachments + 1)},
			wantErr: ErrTooManyAttachments,
		},
		{
			name: "attachment without filename",
			request: SendRequest{From: baseRequest.From, To: baseRequest.To, Attachments: []Attachment{{
				Content: []byte("x"),
			}}},
			wantErr: ErrInvalidAttachment,
		},
		{
			name:    "empty explicit idempotency key",
			request: baseRequest,
			options: []SendOption{WithIdempotencyKey("")},
			wantErr: ErrInvalidIdempotencyKey,
		},
		{
			name:    "oversized idempotency key",
			request: baseRequest,
			options: []SendOption{WithIdempotencyKey(strings.Repeat("k", MaxIdempotencyKeyLength+1))},
			wantErr: ErrInvalidIdempotencyKey,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := client.Email.Send(context.Background(), test.request, test.options...)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("Send() error = %v, want %v", err, test.wantErr)
			}
		})
	}
	if got, want := requests.Load(), int32(1); got != want {
		t.Fatalf("request count after invalid requests = %d, want %d", got, want)
	}
}

func makeTestObject(size int) map[string]any {
	result := make(map[string]any, size)
	for index := 0; index < size; index++ {
		result[fmt.Sprintf("key_%03d", index)] = index
	}
	return result
}

func makeTestAttachments(size int) []Attachment {
	result := make([]Attachment, size)
	for index := range result {
		result[index] = Attachment{Filename: fmt.Sprintf("attachment-%02d.txt", index), Content: []byte("x")}
	}
	return result
}

func TestSDKTransport_ClonesRequestBeforeSettingHeaders(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "https://api.example.com/v1/usage", nil)
	request.Header.Set("User-Agent", "caller/1.0")
	request.Header.Set("Cookie", "viapost_session=must-not-leak")
	transport := &sdkTransport{
		userAgent: "viapost-go/test",
		origin:    "https://api.example.com:443",
		next: roundTripperFunc(func(got *http.Request) (*http.Response, error) {
			if userAgent := got.Header.Get("User-Agent"); userAgent != "viapost-go/test" {
				t.Errorf("transport User-Agent = %q, want viapost-go/test", userAgent)
			}
			if cookie := got.Header.Get("Cookie"); cookie != "" {
				t.Errorf("transport Cookie = %q, want empty", cookie)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       http.NoBody,
				Request:    got,
			}, nil
		}),
	}

	if _, err := transport.RoundTrip(request); err != nil {
		t.Fatalf("RoundTrip() error = %v", err)
	}
	if got, want := request.Header.Get("User-Agent"), "caller/1.0"; got != want {
		t.Fatalf("original request User-Agent = %q, want %q", got, want)
	}
	if got, want := request.Header.Get("Cookie"), "viapost_session=must-not-leak"; got != want {
		t.Fatalf("original request Cookie = %q, want %q", got, want)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestClient_DoesNotFollowRedirectsWithBearerCredentials(t *testing.T) {
	var redirectedRequests atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		redirectedRequests.Add(1)
	}))
	defer target.Close()

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, &http.Request{}, target.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()

	client, err := NewClient("vp_test_example", WithBaseURL(source.URL))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if _, err := client.Usage.Get(context.Background()); err == nil {
		t.Fatal("Usage.Get() error = nil, want redirect rejection")
	}
	if got := redirectedRequests.Load(); got != 0 {
		t.Fatalf("redirect target request count = %d, want 0", got)
	}
}

func TestClient_RejectsOversizedSuccessResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(make([]byte, maxSuccessResponseBytes+1))
	}))
	defer server.Close()

	client, err := NewClient("vp_test_example", WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	_, err = client.Usage.Get(context.Background())
	if !errors.Is(err, ErrResponseBodyTooLarge) {
		t.Fatalf("Usage.Get() error = %v, want ErrResponseBodyTooLarge", err)
	}
}

func TestClient_AllowsRawMessageLargerThanJSONLimit(t *testing.T) {
	const messageID = "018f0000-0000-7000-8000-000000000001"
	body := make([]byte, maxSuccessResponseBytes+1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/v1/messages/"+messageID+"/raw"; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		w.Header().Set("Content-Type", "message/rfc822")
		_, _ = w.Write(body)
	}))
	defer server.Close()

	client, err := NewClient("vp_test_example", WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	response, err := client.Raw().GetMessagesIDRaw(context.Background(), api.GetMessagesIDRawParams{ID: messageID})
	if err != nil {
		t.Fatalf("GetMessagesIDRaw() error = %v", err)
	}
	raw, ok := response.(*api.GetMessagesIDRawOKHeaders)
	if !ok {
		t.Fatalf("GetMessagesIDRaw() response = %T, want *api.GetMessagesIDRawOKHeaders", response)
	}
	decoded, err := io.ReadAll(raw.Response)
	if err != nil {
		t.Fatalf("read raw response: %v", err)
	}
	if got, want := len(decoded), len(body); got != want {
		t.Fatalf("raw response length = %d, want %d", got, want)
	}
}

func TestClient_RejectsRawMessageAboveConfiguredLimit(t *testing.T) {
	const (
		messageID = "018f0000-0000-7000-8000-000000000001"
		limit     = 1 << 20
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "message/rfc822")
		_, _ = w.Write(make([]byte, limit+1))
	}))
	defer server.Close()

	client, err := NewClient(
		"vp_test_example",
		WithBaseURL(server.URL),
		WithMaxRawResponseBytes(limit),
	)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	_, err = client.Raw().GetMessagesIDRaw(context.Background(), api.GetMessagesIDRawParams{ID: messageID})
	if !errors.Is(err, ErrResponseBodyTooLarge) {
		t.Fatalf("GetMessagesIDRaw() error = %v, want ErrResponseBodyTooLarge", err)
	}
}

func TestClient_RejectsInvalidRawResponseLimit(t *testing.T) {
	_, err := NewClient("vp_test_example", WithMaxRawResponseBytes(0))
	if !errors.Is(err, ErrInvalidResponseBodyLimit) {
		t.Fatalf("NewClient() error = %v, want ErrInvalidResponseBodyLimit", err)
	}
}

func TestClient_TruncatesOversizedAPIErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Request-ID", "req-large-error")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(strings.Repeat("x", maxErrorBodyBytes+128)))
	}))
	defer server.Close()

	client, err := NewClient("vp_test_example", WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	_, err = client.Usage.Get(context.Background())
	var apiError *APIError
	if !errors.As(err, &apiError) {
		t.Fatalf("Usage.Get() error = %T %v, want *APIError", err, err)
	}
	if !apiError.Truncated {
		t.Fatal("APIError.Truncated = false, want true")
	}
	if got, want := string(apiError.Body), `{"error":{"message":"response body truncated by SDK safety limit"}}`; got != want {
		t.Fatalf("APIError body = %q, want fail-closed body %q", got, want)
	}
	if apiError.RequestID != "req-large-error" {
		t.Fatalf("APIError.RequestID = %q, want req-large-error", apiError.RequestID)
	}
}

func TestClient_RedactsAPIKeyFromErrorResponse(t *testing.T) {
	const apiKey = "vp_test_super_secret_value"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Request-ID", "request-"+apiKey)
		w.Header().Set("X-Debug", "Bearer "+apiKey)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":"invalid_key","message":"received ` + apiKey + `"}}`))
	}))
	defer server.Close()

	client, err := NewClient(apiKey, WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	_, err = client.Usage.Get(context.Background())
	var apiError *APIError
	if !errors.As(err, &apiError) {
		t.Fatalf("Usage.Get() error = %T %v, want *APIError", err, err)
	}
	for label, value := range map[string]string{
		"Error()":   apiError.Error(),
		"Body":      string(apiError.Body),
		"RequestID": apiError.RequestID,
		"Header":    fmt.Sprint(apiError.Header),
	} {
		if strings.Contains(value, apiKey) {
			t.Fatalf("%s contains API key: %q", label, value)
		}
	}
}
