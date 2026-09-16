package viapost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ViaPost-io/viapost-go/api"
)

const (
	// Version is the semantic version of this SDK.
	Version = "0.2.0"
	// DefaultBaseURL is the production ViaPost API endpoint.
	DefaultBaseURL = "https://api.viapost.io"
	// DefaultTimeout bounds a request when the supplied context has no earlier deadline.
	DefaultTimeout          = 60 * time.Second
	defaultUserAgent        = "viapost-go/" + Version
	maxErrorBodyBytes       = 1 << 20
	maxSuccessResponseBytes = 8 << 20
)

var (
	ErrMissingAPIKey        = errors.New("viapost: API key is required")
	ErrInvalidBaseURL       = errors.New("viapost: base URL must be an absolute HTTP(S) URL without credentials, query, or fragment")
	ErrInvalidTimeout       = errors.New("viapost: timeout must be greater than zero")
	ErrInsecureBaseURL      = errors.New("viapost: plain HTTP base URLs are allowed only for explicit loopback hosts")
	ErrCrossOriginRequest   = errors.New("viapost: refusing to send API credentials to a different origin")
	ErrResponseBodyTooLarge = errors.New("viapost: response body exceeds the 8 MiB safety limit")
)

type config struct {
	baseURL   string
	timeout   time.Duration
	userAgent string
	http      *http.Client
}

// Option customizes a Client.
type Option func(*config) error

// WithBaseURL directs requests to a custom ViaPost-compatible endpoint.
func WithBaseURL(baseURL string) Option {
	return func(cfg *config) error {
		cfg.baseURL = baseURL
		return nil
	}
}

// WithTimeout changes the default request timeout.
func WithTimeout(timeout time.Duration) Option {
	return func(cfg *config) error {
		cfg.timeout = timeout
		return nil
	}
}

// WithUserAgent changes the SDK identifier sent with requests.
func WithUserAgent(userAgent string) Option {
	return func(cfg *config) error {
		cfg.userAgent = userAgent
		return nil
	}
}

// WithHTTPClient uses a copy of client for transport and TLS configuration.
// Cookie jars and redirect policies are intentionally replaced to keep API-key
// authentication isolated to the configured origin.
func WithHTTPClient(client *http.Client) Option {
	return func(cfg *config) error {
		cfg.http = client
		return nil
	}
}

// Client is a server-side ViaPost API client.
type Client struct {
	raw *api.Client

	Email       *EmailService
	Messages    *MessagesService
	Domains     *DomainsService
	Templates   *TemplatesService
	Webhooks    *WebhooksService
	Automations *AutomationsService
	Usage       *UsageService
}

// NewClient creates a server-side client authenticated with a tenant API key.
func NewClient(apiKey string, options ...Option) (*Client, error) {
	if apiKey == "" || apiKey != strings.TrimSpace(apiKey) {
		return nil, ErrMissingAPIKey
	}

	cfg := config{
		baseURL:   DefaultBaseURL,
		timeout:   DefaultTimeout,
		userAgent: defaultUserAgent,
	}
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option(&cfg); err != nil {
			return nil, err
		}
	}
	parsedBaseURL, err := url.Parse(cfg.baseURL)
	if err != nil || (parsedBaseURL.Scheme != "http" && parsedBaseURL.Scheme != "https") || parsedBaseURL.Host == "" || parsedBaseURL.User != nil || parsedBaseURL.RawQuery != "" || parsedBaseURL.Fragment != "" {
		return nil, ErrInvalidBaseURL
	}
	if parsedBaseURL.Scheme == "http" && !isLoopbackHost(parsedBaseURL.Hostname()) {
		return nil, ErrInsecureBaseURL
	}
	if cfg.timeout <= 0 {
		return nil, ErrInvalidTimeout
	}

	httpClient := &http.Client{}
	if cfg.http != nil {
		clone := *cfg.http
		httpClient = &clone
	}
	httpClient.Jar = nil
	httpClient.Timeout = cfg.timeout
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	httpClient.Transport = &sdkTransport{
		next:      httpClient.Transport,
		userAgent: cfg.userAgent,
		origin:    normalizedOrigin(parsedBaseURL),
	}

	raw, err := api.NewClient(cfg.baseURL, bearerSecuritySource{apiKey: apiKey}, api.WithClient(httpClient))
	if err != nil {
		return nil, err
	}
	client := &Client{raw: raw}
	client.Email = &EmailService{client: client}
	client.Messages = &MessagesService{client: client}
	client.Domains = &DomainsService{client: client}
	client.Templates = &TemplatesService{client: client}
	client.Webhooks = &WebhooksService{client: client}
	client.Automations = &AutomationsService{client: client}
	client.Usage = &UsageService{client: client}
	return client, nil
}

// Raw exposes the complete generated OpenAPI client for advanced endpoints.
func (c *Client) Raw() *api.Client { return c.raw }

type bearerSecuritySource struct{ apiKey string }

func (s bearerSecuritySource) BearerAPIKey(context.Context, api.OperationName) (api.BearerAPIKey, error) {
	return api.BearerAPIKey{Token: s.apiKey}, nil
}

type sdkTransport struct {
	next      http.RoundTripper
	userAgent string
	origin    string
}

func (t *sdkTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil || normalizedOrigin(request.URL) != t.origin {
		return nil, ErrCrossOriginRequest
	}
	next := t.next
	if next == nil {
		next = http.DefaultTransport
	}
	request = request.Clone(request.Context())
	request.Header.Del("Cookie")
	request.Header.Set("User-Agent", t.userAgent)
	response, err := next.RoundTrip(request)
	if err != nil {
		return response, err
	}
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		body, readErr := io.ReadAll(io.LimitReader(response.Body, maxSuccessResponseBytes+1))
		_ = response.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("viapost: read response: %w", readErr)
		}
		if len(body) > maxSuccessResponseBytes {
			return nil, ErrResponseBodyTooLarge
		}
		response.Body = io.NopCloser(bytes.NewReader(body))
		response.ContentLength = int64(len(body))
		return response, nil
	}

	body, readErr := io.ReadAll(io.LimitReader(response.Body, maxErrorBodyBytes+1))
	_ = response.Body.Close()
	if readErr != nil {
		return nil, fmt.Errorf("viapost: read error response: %w", readErr)
	}
	apiError := newAPIError(response, body)
	return nil, apiError
}

func normalizedOrigin(target *url.URL) string {
	if target == nil {
		return ""
	}
	port := target.Port()
	if port == "" {
		switch strings.ToLower(target.Scheme) {
		case "http":
			port = "80"
		case "https":
			port = "443"
		}
	}
	if port == "" || target.Hostname() == "" {
		return ""
	}
	host := net.JoinHostPort(strings.ToLower(target.Hostname()), port)
	return strings.ToLower(target.Scheme) + "://" + host
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(strings.TrimSuffix(host, "."), "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// APIError describes a non-2xx response returned by the ViaPost API.
type APIError struct {
	StatusCode int
	Code       string
	Message    string
	RequestID  string
	Body       []byte
	Header     http.Header
	Truncated  bool
}

func (e *APIError) Error() string {
	if e.Code != "" && e.Message != "" {
		return fmt.Sprintf("viapost: HTTP %d %s: %s", e.StatusCode, e.Code, e.Message)
	}
	if e.Message != "" {
		return fmt.Sprintf("viapost: HTTP %d: %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("viapost: HTTP %d", e.StatusCode)
}

func newAPIError(response *http.Response, body []byte) *APIError {
	truncated := len(body) > maxErrorBodyBytes
	if truncated {
		body = body[:maxErrorBodyBytes]
	}
	result := &APIError{
		StatusCode: response.StatusCode,
		RequestID:  response.Header.Get("X-Request-ID"),
		Body:       append([]byte(nil), body...),
		Header:     response.Header.Clone(),
		Truncated:  truncated,
	}
	var envelope struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &envelope) == nil {
		result.Code = envelope.Error.Code
		result.Message = envelope.Error.Message
		if result.RequestID == "" {
			result.RequestID = envelope.Error.RequestID
		}
	}
	return result
}
