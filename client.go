package viapost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ViaPost-io/viapost-go/api"
)

const (
	// Version is the semantic version of this SDK.
	Version = "0.3.0"
	// DefaultBaseURL is the production ViaPost API endpoint.
	DefaultBaseURL = "https://api.viapost.io"
	// DefaultPublicStatusURL is the canonical unauthenticated status endpoint.
	DefaultPublicStatusURL = "https://status.viapost.io"
	// DefaultTimeout bounds a request when the supplied context has no earlier deadline.
	DefaultTimeout = 60 * time.Second
	// DefaultMaxRawResponseBytes accepts the largest raw message supported by
	// the public API while JSON responses keep their smaller defensive limit.
	DefaultMaxRawResponseBytes      int64 = 40 << 20
	defaultUserAgent                      = "viapost-go/" + Version
	maxErrorBodyBytes                     = 1 << 20
	maxSuccessResponseBytes               = 8 << 20
	maxConfigurableRawResponseBytes int64 = 128 << 20
)

var (
	ErrMissingAPIKey            = errors.New("viapost: API key is required")
	ErrInvalidBaseURL           = errors.New("viapost: base URL must be an absolute HTTP(S) URL without credentials, query, or fragment")
	ErrInvalidTimeout           = errors.New("viapost: timeout must be greater than zero")
	ErrInsecureBaseURL          = errors.New("viapost: plain HTTP base URLs are allowed only for explicit loopback hosts")
	ErrCrossOriginRequest       = errors.New("viapost: refusing to send API credentials to a different origin")
	ErrResponseBodyTooLarge     = errors.New("viapost: response body exceeds the configured safety limit")
	ErrInvalidResponseBodyLimit = errors.New("viapost: raw response body limit must be between 1 byte and 128 MiB")
	ErrPublicStatusOnly         = errors.New("viapost: authenticated operations are disabled on the public status client")
)

type config struct {
	baseURL             string
	timeout             time.Duration
	userAgent           string
	http                *http.Client
	maxRawResponseBytes int64
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

// WithMaxRawResponseBytes changes the maximum buffered size for raw RFC822
// messages and CSV exports. JSON and error responses retain their smaller,
// fixed defensive limits.
func WithMaxRawResponseBytes(limit int64) Option {
	return func(cfg *config) error {
		if limit <= 0 || limit > maxConfigurableRawResponseBytes {
			return ErrInvalidResponseBodyLimit
		}
		cfg.maxRawResponseBytes = limit
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
		baseURL:             DefaultBaseURL,
		timeout:             DefaultTimeout,
		userAgent:           defaultUserAgent,
		maxRawResponseBytes: DefaultMaxRawResponseBytes,
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
		next:                httpClient.Transport,
		userAgent:           cfg.userAgent,
		origin:              normalizedOrigin(parsedBaseURL),
		maxRawResponseBytes: cfg.maxRawResponseBytes,
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

// NewPublicStatusClient creates an unauthenticated client for the operations
// served only by status.viapost.io. Options such as WithBaseURL and
// WithHTTPClient may be used for local testing.
func NewPublicStatusClient(options ...Option) (*api.Client, error) {
	cfg := config{
		baseURL:             DefaultPublicStatusURL,
		timeout:             DefaultTimeout,
		userAgent:           defaultUserAgent,
		maxRawResponseBytes: DefaultMaxRawResponseBytes,
	}
	for _, option := range options {
		if option != nil {
			if err := option(&cfg); err != nil {
				return nil, err
			}
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
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	httpClient.Transport = &sdkTransport{
		next:                httpClient.Transport,
		userAgent:           cfg.userAgent,
		origin:              normalizedOrigin(parsedBaseURL),
		maxRawResponseBytes: cfg.maxRawResponseBytes,
	}
	return api.NewClient(cfg.baseURL, publicStatusSecuritySource{}, api.WithClient(httpClient))
}

// Raw exposes the complete generated OpenAPI client for advanced endpoints.
func (c *Client) Raw() *api.Client { return c.raw }

func (c Client) String() string   { return "ViaPostClient{Credentials:[REDACTED]}" }
func (c Client) GoString() string { return c.String() }
func (c Client) LogValue() slog.Value {
	return slog.GroupValue(slog.String("credentials", "[REDACTED]"))
}

type bearerSecuritySource struct{ apiKey string }

func (s bearerSecuritySource) String() string   { return "BearerSecuritySource{APIKey:[REDACTED]}" }
func (s bearerSecuritySource) GoString() string { return s.String() }
func (s bearerSecuritySource) LogValue() slog.Value {
	return slog.GroupValue(slog.String("api_key", "[REDACTED]"))
}

func (s bearerSecuritySource) BearerAPIKey(context.Context, api.OperationName) (api.BearerAPIKey, error) {
	return api.BearerAPIKey{Token: s.apiKey}, nil
}

type publicStatusSecuritySource struct{}

func (publicStatusSecuritySource) BearerAPIKey(context.Context, api.OperationName) (api.BearerAPIKey, error) {
	return api.BearerAPIKey{}, ErrPublicStatusOnly
}

type sdkTransport struct {
	next                http.RoundTripper
	userAgent           string
	origin              string
	maxRawResponseBytes int64
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
		limit := int64(maxSuccessResponseBytes)
		if isRawResponse(request, response) {
			limit = t.maxRawResponseBytes
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, limit+1))
		_ = response.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("viapost: read response: %w", readErr)
		}
		if int64(len(body)) > limit {
			return nil, ErrResponseBodyTooLarge
		}
		response.Body = io.NopCloser(bytes.NewReader(body))
		response.ContentLength = int64(len(body))
		return response, nil
	}

	body, readErr := io.ReadAll(io.LimitReader(response.Body, int64(maxErrorBodyBytes+1)))
	_ = response.Body.Close()
	if readErr != nil {
		return nil, fmt.Errorf("viapost: read error response: %w", readErr)
	}
	apiError := newAPIError(response, body, bearerCredential(request.Header.Get("Authorization")))
	return nil, apiError
}

func isRawResponse(request *http.Request, response *http.Response) bool {
	if request != nil && request.URL != nil && strings.HasSuffix(strings.TrimRight(request.URL.Path, "/"), "/raw") {
		return true
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil {
		return false
	}
	return mediaType == "message/rfc822" || mediaType == "text/csv"
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

func newAPIError(response *http.Response, body []byte, credential string) *APIError {
	truncated := len(body) > maxErrorBodyBytes
	if truncated {
		body = []byte(`{"error":{"message":"response body truncated by SDK safety limit"}}`)
	}
	body, sensitiveValues := redactSensitiveJSON(body, credential)
	header := response.Header.Clone()
	for name, values := range header {
		if isSensitiveName(name) {
			for _, value := range values {
				if value != "" {
					sensitiveValues = append(sensitiveValues, value)
				}
			}
		}
	}
	for name, values := range header {
		for index, value := range values {
			if isSensitiveName(name) {
				values[index] = "[REDACTED]"
			} else {
				values[index] = redactStrings(value, sensitiveValues)
			}
		}
		header[name] = values
	}
	result := &APIError{
		StatusCode: response.StatusCode,
		RequestID:  header.Get("X-Request-ID"),
		Body:       append([]byte(nil), body...),
		Header:     header,
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

func bearerCredential(authorization string) string {
	scheme, credential, found := strings.Cut(authorization, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(credential)
}

func redactBytes(value []byte, secret string) []byte {
	if secret == "" {
		return value
	}
	return bytes.ReplaceAll(value, []byte(secret), []byte("[REDACTED]"))
}

func redactString(value, secret string) string {
	if secret == "" {
		return value
	}
	return strings.ReplaceAll(value, secret, "[REDACTED]")
}

func redactSensitiveJSON(body []byte, credential string) ([]byte, []string) {
	secrets := []string{}
	if credential != "" {
		secrets = append(secrets, credential)
	}
	var value any
	if json.Unmarshal(body, &value) != nil {
		return redactBytes(body, credential), secrets
	}
	collectNamedSensitiveValues(value, &secrets)
	value = redactJSONValue(value, secrets)
	redacted, err := json.Marshal(value)
	if err != nil {
		return redactBytes(body, credential), secrets
	}
	return redacted, secrets
}

func collectNamedSensitiveValues(value any, secrets *[]string) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if isSensitiveName(key) {
				collectStrings(child, secrets)
				continue
			}
			collectNamedSensitiveValues(child, secrets)
		}
	case []any:
		for _, child := range typed {
			collectNamedSensitiveValues(child, secrets)
		}
	}
}

func collectStrings(value any, secrets *[]string) {
	switch typed := value.(type) {
	case string:
		if typed != "" {
			*secrets = append(*secrets, typed)
		}
	case map[string]any:
		for _, child := range typed {
			collectStrings(child, secrets)
		}
	case []any:
		for _, child := range typed {
			collectStrings(child, secrets)
		}
	}
}

func redactJSONValue(value any, secrets []string) any {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if isSensitiveName(key) {
				typed[key] = "[REDACTED]"
			} else {
				typed[key] = redactJSONValue(child, secrets)
			}
		}
		return typed
	case []any:
		for index, child := range typed {
			typed[index] = redactJSONValue(child, secrets)
		}
		return typed
	case string:
		return redactStrings(typed, secrets)
	default:
		return value
	}
}

func redactStrings(value string, secrets []string) string {
	for _, secret := range secrets {
		value = redactString(value, secret)
	}
	return value
}

func isSensitiveName(name string) bool {
	normalized := strings.NewReplacer("-", "_", " ", "_").Replace(strings.ToLower(name))
	for _, part := range []string{"secret", "token", "password", "api_key", "authorization", "cookie"} {
		if normalized == part || strings.HasSuffix(normalized, "_"+part) {
			return true
		}
	}
	return false
}
