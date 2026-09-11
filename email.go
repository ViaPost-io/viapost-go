package viapost

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ViaPost-io/viapost-go/api"
	"github.com/go-faster/jx"
	"github.com/google/uuid"
)

// Stream selects the transactional or marketing delivery pipeline.
type Stream string

const (
	StreamTransactional Stream = "transactional"
	StreamMarketing     Stream = "marketing"
)

// SendRequest contains the common fields used to send an email.
type SendRequest struct {
	From        string
	FromName    string
	ReplyTo     string
	To          []string
	CC          []string
	BCC         []string
	Subject     string
	HTML        string
	Text        string
	Stream      Stream
	Tags        []string
	Metadata    map[string]any
	TemplateID  *string
	Variables   map[string]any
	Attachments []Attachment
}

// Attachment is an in-memory attachment. Content is encoded as base64 in JSON.
type Attachment struct {
	Filename    string
	Content     []byte
	ContentType string
}

// AcceptedMessage identifies an accepted recipient.
type AcceptedMessage struct {
	MessageID string
	To        string
}

// RejectedMessage identifies a recipient rejected before queueing.
type RejectedMessage struct {
	To     string
	Reason string
}

// SendResult is the recipient-level result returned by ViaPost.
type SendResult struct {
	Accepted []AcceptedMessage
	Rejected []RejectedMessage
}

type sendConfig struct {
	idempotencyKey    string
	idempotencyKeySet bool
}

// SendOption customizes one email send.
type SendOption func(*sendConfig)

// WithIdempotencyKey makes retries of the same logical send safe.
func WithIdempotencyKey(key string) SendOption {
	return func(cfg *sendConfig) {
		cfg.idempotencyKey = key
		cfg.idempotencyKeySet = true
	}
}

// EmailService groups email submission operations.
type EmailService struct{ client *Client }

// Send queues an email for one or more recipients.
func (s *EmailService) Send(ctx context.Context, request SendRequest, options ...SendOption) (*SendResult, error) {
	cfg := sendConfig{}
	for _, option := range options {
		if option != nil {
			option(&cfg)
		}
	}
	if err := validateSendRequest(request, cfg); err != nil {
		return nil, err
	}

	params := api.PostSendParams{}
	if cfg.idempotencyKey != "" {
		params.IdempotencyKey = api.NewOptString(cfg.idempotencyKey)
	}
	apiRequest, err := toAPISendRequest(request)
	if err != nil {
		return nil, err
	}
	response, err := s.client.raw.PostSend(ctx, apiRequest, params)
	if err != nil {
		return nil, err
	}
	result, ok := response.(*api.SendResult)
	if !ok {
		return nil, fmt.Errorf("viapost: unexpected POST /v1/send response %T", response)
	}

	converted := &SendResult{
		Accepted: make([]AcceptedMessage, len(result.Accepted)),
		Rejected: make([]RejectedMessage, len(result.Rejected)),
	}
	for index, accepted := range result.Accepted {
		converted.Accepted[index] = AcceptedMessage{MessageID: uuid.UUID(accepted.MessageID).String(), To: accepted.To}
	}
	for index, rejected := range result.Rejected {
		converted.Rejected[index] = RejectedMessage{To: rejected.To, Reason: string(rejected.Reason)}
	}
	return converted, nil
}

func toAPISendRequest(request SendRequest) (*api.SendRequest, error) {
	converted := &api.SendRequest{
		From:        request.From,
		To:          request.To,
		Cc:          request.CC,
		Bcc:         request.BCC,
		Tags:        request.Tags,
		Attachments: make([]api.Attachment, len(request.Attachments)),
	}
	if request.FromName != "" {
		converted.FromName = api.NewOptString(request.FromName)
	}
	if request.ReplyTo != "" {
		converted.ReplyTo = api.NewOptString(request.ReplyTo)
	}
	if request.Subject != "" {
		converted.Subject = api.NewOptString(request.Subject)
	}
	if request.HTML != "" {
		converted.HTML = api.NewOptString(request.HTML)
	}
	if request.Text != "" {
		converted.Text = api.NewOptString(request.Text)
	}
	if request.Stream != "" {
		converted.Stream = api.NewOptSendRequestStream(api.SendRequestStream(request.Stream))
	}
	if request.TemplateID != nil {
		templateID, err := uuid.Parse(*request.TemplateID)
		if err != nil {
			return nil, ErrInvalidTemplateID
		}
		converted.TemplateID = api.NewOptNilUUID(api.UUID(templateID))
	}
	if request.Metadata != nil {
		metadata, err := encodeOpaqueObject(request.Metadata)
		if err != nil {
			return nil, fmt.Errorf("viapost: encode metadata: %w", err)
		}
		converted.Metadata = api.NewOptSendRequestMetadata(api.SendRequestMetadata(metadata))
	}
	if request.Variables != nil {
		variables, err := encodeOpaqueObject(request.Variables)
		if err != nil {
			return nil, fmt.Errorf("viapost: encode variables: %w", err)
		}
		converted.Variables = api.NewOptSendRequestVariables(api.SendRequestVariables(variables))
	}
	for index, attachment := range request.Attachments {
		converted.Attachments[index] = api.Attachment{
			Filename: attachment.Filename,
			Content:  attachment.Content,
		}
		if attachment.ContentType != "" {
			converted.Attachments[index].ContentType = api.NewOptString(attachment.ContentType)
		}
	}
	return converted, nil
}

func encodeOpaqueObject(value map[string]any) (map[string]jx.Raw, error) {
	encoded := make(map[string]jx.Raw, len(value))
	for key, item := range value {
		data, err := json.Marshal(item)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", key, err)
		}
		encoded[key] = jx.Raw(data)
	}
	return encoded, nil
}
