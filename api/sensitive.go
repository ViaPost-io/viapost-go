package api

import "log/slog"

const redactedSensitiveValue = "[REDACTED]"

func (s CreateWebhookResponse) String() string   { return "CreateWebhookResponse{Secret:[REDACTED]}" }
func (s CreateWebhookResponse) GoString() string { return s.String() }
func (s CreateWebhookResponse) LogValue() slog.Value {
	return slog.GroupValue(slog.String("secret", redactedSensitiveValue))
}

func (s RotateWebhookSecretResponse) String() string {
	return "RotateWebhookSecretResponse{Secret:[REDACTED]}"
}
func (s RotateWebhookSecretResponse) GoString() string { return s.String() }
func (s RotateWebhookSecretResponse) LogValue() slog.Value {
	return slog.GroupValue(slog.String("secret", redactedSensitiveValue))
}

func (c Client) String() string   { return "ViaPostOpenAPIClient{Security:[REDACTED]}" }
func (c Client) GoString() string { return c.String() }
func (c Client) LogValue() slog.Value {
	return slog.GroupValue(slog.String("security", redactedSensitiveValue))
}
