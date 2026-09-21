package viapost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ViaPost-io/viapost-go/api"
)

func TestSensitiveWebhookResponsesAreRedactedByDefault(t *testing.T) {
	const secret = "whsec_never_log_this"
	generated := api.CreateWebhookResponse{Secret: secret}
	rotated := api.RotateWebhookSecretResponse{Secret: api.NewOptString(secret)}
	stable := CreateWebhookResult{secret: secret}

	for label, value := range map[string]any{
		"generated create": generated,
		"generated rotate": rotated,
		"stable":           stable,
	} {
		serialized, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("json.Marshal(%s): %v", label, err)
		}
		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		logger.Info("response", "value", value)
		for representation, output := range map[string]string{
			"json": string(serialized),
			"fmt":  fmt.Sprintf("%+v %#v", value, value),
			"slog": logs.String(),
		} {
			if strings.Contains(output, secret) {
				t.Fatalf("%s %s leaked secret: %s", label, representation, output)
			}
		}
	}
	if stable.Secret() != secret {
		t.Fatal("explicit Secret() accessor did not return the one-time secret")
	}
}

func TestTrackingDomainProofResponsesAreRedactedByDefault(t *testing.T) {
	const proofValue = "vp_tracking_proof_never_log_this"
	proof := api.TrackingDomainProofResponseProof{Name: "_viapost.example.com", Value: proofValue}
	response := api.TrackingDomainProofResponse{Proof: proof}

	for label, value := range map[string]any{
		"proof":    proof,
		"response": response,
	} {
		serialized, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("json.Marshal(%s): %v", label, err)
		}
		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		logger.Info("response", "value", value)
		for representation, output := range map[string]string{
			"json": string(serialized),
			"fmt":  fmt.Sprintf("%+v %#v", value, value),
			"slog": logs.String(),
		} {
			if strings.Contains(output, proofValue) {
				t.Fatalf("%s %s leaked tracking proof: %s", label, representation, output)
			}
		}
	}
	if response.Proof.GetValue() != proofValue {
		t.Fatal("explicit proof accessor did not return the one-time proof value")
	}
}

func TestClientRepresentationsNeverExposeAPIKey(t *testing.T) {
	const apiKey = "vp_test_never_log_this"
	client, err := NewClient(apiKey)
	if err != nil {
		t.Fatal(err)
	}
	for label, value := range map[string]any{"client": client, "raw client": client.Raw()} {
		output := fmt.Sprintf("%+v %#v", value, value)
		if strings.Contains(output, apiKey) {
			t.Fatalf("%s representation leaked API key: %s", label, output)
		}
	}
}

func TestAPIErrorRedactsNamedSensitiveValuesEverywhere(t *testing.T) {
	const secret = "whsec_never_log_this"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Debug", "failed "+secret)
		w.Header().Set("Set-Cookie", "session="+secret)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":"invalid","message":"failed ` + secret + `","secret":"` + secret + `"}}`))
	}))
	defer server.Close()

	client, err := NewClient("vp_test_key", WithBaseURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Usage.Get(context.Background())
	var apiError *APIError
	if !errors.As(err, &apiError) {
		t.Fatalf("error = %T %v, want *APIError", err, err)
	}
	for label, value := range map[string]string{
		"error":  apiError.Error(),
		"body":   string(apiError.Body),
		"header": fmt.Sprint(apiError.Header),
	} {
		if strings.Contains(value, secret) {
			t.Fatalf("%s leaked secret: %s", label, value)
		}
	}
}

func TestWebhookURLRejectsNonPublicDestinations(t *testing.T) {
	for _, target := range []string{
		"https://127.0.0.1/hook",
		"https://127.1/hook",
		"https://2130706433/hook",
		"https://0x7f000001/hook",
		"https://service.localhost/hook",
		"https://192.0.2.1/hook",
		"https://198.51.100.1/hook",
		"https://203.0.113.1/hook",
		"https://[2001:db8::1]/hook",
	} {
		t.Run(target, func(t *testing.T) {
			if _, err := parseWebhookURL(target); !errors.Is(err, ErrInvalidWebhookURL) {
				t.Fatalf("parseWebhookURL(%q) error = %v, want ErrInvalidWebhookURL", target, err)
			}
		})
	}
}

func TestIdempotencyKeyRequiresVisibleASCII(t *testing.T) {
	request := SendRequest{Stream: StreamTransactional}
	for _, key := range []string{"contains space", "unicode-é", "line\nbreak"} {
		if err := validateSendRequest(request, sendConfig{idempotencyKeySet: true, idempotencyKey: key}); !errors.Is(err, ErrInvalidIdempotencyKey) {
			t.Fatalf("key %q error = %v, want ErrInvalidIdempotencyKey", key, err)
		}
	}
}

func TestPublicStatusClientUsesItsDedicatedOriginWithoutCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/status" {
			t.Errorf("path = %q, want /v1/status", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "" {
			t.Errorf("public request included Authorization: %q", request.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewPublicStatusClient(WithBaseURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.HeadPublicStatus(context.Background()); err != nil {
		t.Fatalf("HeadPublicStatus() error = %v", err)
	}
	if _, err := client.GetUsage(context.Background()); !errors.Is(err, ErrPublicStatusOnly) {
		t.Fatalf("authenticated operation error = %v, want ErrPublicStatusOnly", err)
	}
}
