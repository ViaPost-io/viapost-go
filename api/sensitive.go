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

func (s TrackingDomainProofResponse) String() string {
	return "TrackingDomainProofResponse{Proof:[REDACTED]}"
}
func (s TrackingDomainProofResponse) GoString() string { return s.String() }
func (s TrackingDomainProofResponse) LogValue() slog.Value {
	return slog.GroupValue(slog.String("proof", redactedSensitiveValue))
}

func (s TrackingDomainProofResponseProof) String() string {
	return "TrackingDomainProofResponseProof{Value:[REDACTED]}"
}
func (s TrackingDomainProofResponseProof) GoString() string { return s.String() }
func (s TrackingDomainProofResponseProof) LogValue() slog.Value {
	return slog.GroupValue(slog.String("value", redactedSensitiveValue))
}

func (c Client) String() string   { return "ViaPostOpenAPIClient{Security:[REDACTED]}" }
func (c Client) GoString() string { return c.String() }
func (c Client) LogValue() slog.Value {
	return slog.GroupValue(slog.String("security", redactedSensitiveValue))
}
