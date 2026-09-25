package viapost

import (
	"context"
	"fmt"
	"time"

	"github.com/ViaPost-io/viapost-go/api"
)

// MessagePeriod selects a supported relative window for message listings.
type MessagePeriod string

const (
	MessagePeriod24Hours MessagePeriod = "24h"
	MessagePeriod7Days   MessagePeriod = "7d"
	MessagePeriod14Days  MessagePeriod = "14d"
	MessagePeriod30Days  MessagePeriod = "30d"
)

// MessageListOptions controls cursor pagination and filtering.
type MessageListOptions struct {
	Cursor   *time.Time
	Limit    int
	Status   string
	Search   string
	Period   MessagePeriod
	APIKeyID string
}

// MessagesService exposes message logs and delivery metrics.
type MessagesService struct{ client *Client }

// List returns messages matching the supplied filters.
func (s *MessagesService) List(ctx context.Context, options MessageListOptions) ([]Message, error) {
	if !options.Period.valid() {
		return nil, ErrInvalidMessagePeriod
	}
	params := api.GetMessagesParams{}
	if options.Cursor != nil {
		params.Cursor = api.NewOptDateTime(*options.Cursor)
	}
	if options.Limit > 0 {
		params.Limit = api.NewOptInt(options.Limit)
	}
	if options.Status != "" {
		params.Status = api.NewOptString(options.Status)
	}
	if options.Search != "" {
		params.Search = api.NewOptString(options.Search)
	}
	if options.Period != "" {
		params.Period = api.NewOptGetMessagesPeriod(api.GetMessagesPeriod(options.Period))
	}
	if options.APIKeyID != "" {
		params.APIKeyID = api.NewOptString(options.APIKeyID)
	}
	response, err := s.client.raw.GetMessages(ctx, params)
	if err != nil {
		return nil, err
	}
	result, ok := response.(*api.MessageList)
	if !ok {
		return nil, fmt.Errorf("viapost: unexpected GET /v1/messages response %T", response)
	}
	return convertGenerated[[]Message](result.Messages)
}

func (period MessagePeriod) valid() bool {
	switch period {
	case "", MessagePeriod24Hours, MessagePeriod7Days, MessagePeriod14Days, MessagePeriod30Days:
		return true
	default:
		return false
	}
}

// Get returns one message by ID.
func (s *MessagesService) Get(ctx context.Context, id string) (*Message, error) {
	response, err := s.client.raw.GetMessagesID(ctx, api.GetMessagesIDParams{ID: id})
	if err != nil {
		return nil, err
	}
	result, ok := response.(*api.MessageDetail)
	if !ok {
		return nil, fmt.Errorf("viapost: unexpected GET /v1/messages/{id} response %T", response)
	}
	converted, err := convertGenerated[Message](result)
	return &converted, err
}

// Events returns the recorded lifecycle events for a message.
func (s *MessagesService) Events(ctx context.Context, id string) ([]MessageEvent, error) {
	response, err := s.client.raw.GetMessagesIDEvents(ctx, api.GetMessagesIDEventsParams{ID: id})
	if err != nil {
		return nil, err
	}
	result, ok := response.(*api.MessageEventList)
	if !ok {
		return nil, fmt.Errorf("viapost: unexpected GET /v1/messages/{id}/events response %T", response)
	}
	return convertGenerated[[]MessageEvent](result.Events)
}

// Metrics returns aggregate delivery metrics for up to 90 days.
func (s *MessagesService) Metrics(ctx context.Context, days int, domainID string) (*MetricsResponse, error) {
	params := api.GetMessagesMetricsParams{}
	if days > 0 {
		params.Days = api.NewOptInt(days)
	}
	if domainID != "" {
		params.DomainID = api.NewOptString(domainID)
	}
	response, err := s.client.raw.GetMessagesMetrics(ctx, params)
	if err != nil {
		return nil, err
	}
	result, ok := response.(*api.MetricsResponseHeaders)
	if !ok {
		return nil, fmt.Errorf("viapost: unexpected GET /v1/messages/metrics response %T", response)
	}
	converted, err := convertGenerated[MetricsResponse](&result.Response)
	return &converted, err
}

// Engagement returns aggregate delivered, opened, and clicked counts.
func (s *MessagesService) Engagement(ctx context.Context, days int) (*EngagementResult, error) {
	params := api.GetMessagesEngagementParams{}
	if days > 0 {
		params.Days = api.NewOptInt(days)
	}
	response, err := s.client.raw.GetMessagesEngagement(ctx, params)
	if err != nil {
		return nil, err
	}
	result, ok := response.(*api.EngagementResponse)
	if !ok {
		return nil, fmt.Errorf("viapost: unexpected GET /v1/messages/engagement response %T", response)
	}
	converted, err := convertGenerated[EngagementResult](result)
	return &converted, err
}

// Timeseries returns daily message status counts.
func (s *MessagesService) Timeseries(ctx context.Context, days int) (*TimeseriesResult, error) {
	params := api.GetMessagesTimeseriesParams{}
	if days > 0 {
		params.Days = api.NewOptInt(days)
	}
	response, err := s.client.raw.GetMessagesTimeseries(ctx, params)
	if err != nil {
		return nil, err
	}
	result, ok := response.(*api.TimeseriesResponse)
	if !ok {
		return nil, fmt.Errorf("viapost: unexpected GET /v1/messages/timeseries response %T", response)
	}
	converted, err := convertGenerated[TimeseriesResult](result)
	return &converted, err
}
