package viapost

import (
	"context"
	"fmt"
	"time"

	"github.com/ViaPost-io/viapost-go/api"
)

// DomainsService exposes sender-domain operations.
type DomainsService struct{ client *Client }

func (s *DomainsService) List(ctx context.Context) ([]Domain, error) {
	response, err := s.client.raw.GetDomains(ctx)
	if err != nil {
		return nil, err
	}
	result, ok := response.(*api.DomainList)
	if !ok {
		return nil, unexpectedResponse("GET /v1/domains", response)
	}
	return convertGenerated[[]Domain](result.Domains)
}

func (s *DomainsService) Get(ctx context.Context, id string) (*Domain, error) {
	response, err := s.client.raw.GetDomainsID(ctx, api.GetDomainsIDParams{ID: id})
	if err != nil {
		return nil, err
	}
	result, ok := response.(*api.Domain)
	if !ok {
		return nil, unexpectedResponse("GET /v1/domains/{id}", response)
	}
	converted, err := convertGenerated[Domain](result)
	return &converted, err
}

func (s *DomainsService) Create(ctx context.Context, name string) (*Domain, []DNSRecord, error) {
	response, err := s.client.raw.PostDomains(ctx, &api.CreateDomainRequest{Name: name})
	if err != nil {
		return nil, nil, err
	}
	result, ok := response.(*api.CreateDomainResponse)
	if !ok {
		return nil, nil, unexpectedResponse("POST /v1/domains", response)
	}
	domain, err := convertGenerated[Domain](result.Domain)
	if err != nil {
		return nil, nil, err
	}
	records, err := convertGenerated[[]DNSRecord](result.DNSRecords)
	if err != nil {
		return nil, nil, err
	}
	return &domain, records, nil
}

func (s *DomainsService) Delete(ctx context.Context, id string) error {
	_, err := s.client.raw.DeleteDomainsID(ctx, api.DeleteDomainsIDParams{ID: id})
	return err
}

// TemplateListOptions controls template cursor pagination and search.
type TemplateListOptions struct {
	Cursor *time.Time
	Limit  int
	Search string
}

// TemplatesService exposes template lifecycle operations.
type TemplatesService struct{ client *Client }

func (s *TemplatesService) List(ctx context.Context, options TemplateListOptions) ([]EmailTemplate, error) {
	params := api.GetTemplatesParams{}
	if options.Cursor != nil {
		params.Cursor = api.NewOptDateTime(*options.Cursor)
	}
	if options.Limit > 0 {
		params.Limit = api.NewOptInt(options.Limit)
	}
	if options.Search != "" {
		params.Search = api.NewOptString(options.Search)
	}
	response, err := s.client.raw.GetTemplates(ctx, params)
	if err != nil {
		return nil, err
	}
	result, ok := response.(*api.TemplateList)
	if !ok {
		return nil, unexpectedResponse("GET /v1/templates", response)
	}
	return convertGenerated[[]EmailTemplate](result.Templates)
}

func (s *TemplatesService) Get(ctx context.Context, id string) (*EmailTemplate, error) {
	response, err := s.client.raw.GetTemplatesID(ctx, api.GetTemplatesIDParams{ID: id})
	if err != nil {
		return nil, err
	}
	result, ok := response.(*api.EmailTemplate)
	if !ok {
		return nil, unexpectedResponse("GET /v1/templates/{id}", response)
	}
	converted, err := convertGenerated[EmailTemplate](result)
	return &converted, err
}

func (s *TemplatesService) Create(ctx context.Context, name string) (*CreateTemplateResult, error) {
	response, err := s.client.raw.PostTemplates(ctx, &api.CreateTemplateRequest{Name: name})
	if err != nil {
		return nil, err
	}
	result, ok := response.(*api.CreateTemplateResponse)
	if !ok {
		return nil, unexpectedResponse("POST /v1/templates", response)
	}
	converted, err := convertGenerated[CreateTemplateResult](result)
	return &converted, err
}

func (s *TemplatesService) Delete(ctx context.Context, id string) error {
	_, err := s.client.raw.DeleteTemplatesID(ctx, api.DeleteTemplatesIDParams{ID: id})
	return err
}

// WebhooksService exposes outbound webhook endpoint operations.
type WebhooksService struct{ client *Client }

func (s *WebhooksService) List(ctx context.Context) ([]WebhookEndpoint, error) {
	response, err := s.client.raw.GetWebhooks(ctx)
	if err != nil {
		return nil, err
	}
	result, ok := response.(*api.WebhookList)
	if !ok {
		return nil, unexpectedResponse("GET /v1/webhooks", response)
	}
	return convertGenerated[[]WebhookEndpoint](result.Webhooks)
}

func (s *WebhooksService) Create(ctx context.Context, endpointURL string, eventTypes []string) (*CreateWebhookResult, error) {
	parsedURL, err := parseWebhookURL(endpointURL)
	if err != nil {
		return nil, err
	}
	generatedEventTypes := make([]api.WebhookSubscribableEventType, len(eventTypes))
	for index, eventType := range eventTypes {
		generatedEventTypes[index] = api.WebhookSubscribableEventType(eventType)
	}
	response, err := s.client.raw.PostWebhooks(ctx, &api.CreateWebhookRequest{URL: *parsedURL, EventTypes: generatedEventTypes})
	if err != nil {
		return nil, err
	}
	result, ok := response.(*api.CreateWebhookResponse)
	if !ok {
		return nil, unexpectedResponse("POST /v1/webhooks", response)
	}
	converted, err := convertGenerated[CreateWebhookResult](result)
	return &converted, err
}

func (s *WebhooksService) Delete(ctx context.Context, id string) error {
	_, err := s.client.raw.DeleteWebhooksID(ctx, api.DeleteWebhooksIDParams{ID: id})
	return err
}

// AutomationListOptions filters automations by status or name.
type AutomationListOptions struct {
	Status AutomationStatus
	Search string
}

// AutomationStatus selects one lifecycle state for automation listings.
type AutomationStatus string

const (
	AutomationStatusDisabled AutomationStatus = "disabled"
	AutomationStatusEnabled  AutomationStatus = "enabled"
	AutomationStatusArchived AutomationStatus = "archived"
)

// AutomationsService exposes automation definitions.
type AutomationsService struct{ client *Client }

func (s *AutomationsService) List(ctx context.Context, options AutomationListOptions) ([]Automation, error) {
	if !options.Status.valid() {
		return nil, ErrInvalidAutomationStatus
	}
	params := api.GetAutomationsParams{}
	if options.Status != "" {
		params.Status = api.NewOptGetAutomationsStatus(api.GetAutomationsStatus(options.Status))
	}
	if options.Search != "" {
		params.Search = api.NewOptString(options.Search)
	}
	response, err := s.client.raw.GetAutomations(ctx, params)
	if err != nil {
		return nil, err
	}
	result, ok := response.(*api.AutomationList)
	if !ok {
		return nil, unexpectedResponse("GET /v1/automations", response)
	}
	return convertAutomations(result.Data)
}

func (status AutomationStatus) valid() bool {
	switch status {
	case "", AutomationStatusDisabled, AutomationStatusEnabled, AutomationStatusArchived:
		return true
	default:
		return false
	}
}

func (s *AutomationsService) Get(ctx context.Context, id string) (*Automation, error) {
	response, err := s.client.raw.GetAutomationsID(ctx, api.GetAutomationsIDParams{ID: id})
	if err != nil {
		return nil, err
	}
	result, ok := response.(*api.Automation)
	if !ok {
		return nil, unexpectedResponse("GET /v1/automations/{id}", response)
	}
	converted, err := convertAutomation(*result)
	return &converted, err
}

func (s *AutomationsService) Create(ctx context.Context, name string) (*Automation, error) {
	response, err := s.client.raw.PostAutomations(ctx, &api.CreateAutomationRequest{Name: name})
	if err != nil {
		return nil, err
	}
	result, ok := response.(*api.AutomationHeaders)
	if !ok {
		return nil, unexpectedResponse("POST /v1/automations", response)
	}
	converted, err := convertAutomation(result.Response)
	return &converted, err
}

func convertAutomations(source []api.Automation) ([]Automation, error) {
	result := make([]Automation, len(source))
	for index, generated := range source {
		converted, err := convertAutomation(generated)
		if err != nil {
			return nil, err
		}
		result[index] = converted
	}
	return result, nil
}

func convertAutomation(source api.Automation) (Automation, error) {
	// The field is optional and nullable in the public contract. ogen's JSON
	// marshaler rejects an omitted OptNilUUID, so normalize omission to the
	// equivalent null value before mapping to the stable facade model.
	if source.CurrentVersionID.IsEmpty() {
		source.CurrentVersionID.SetToNull()
	}
	return convertGenerated[Automation](source)
}

func (s *AutomationsService) Delete(ctx context.Context, id string) error {
	_, err := s.client.raw.DeleteAutomationsID(ctx, api.DeleteAutomationsIDParams{ID: id})
	return err
}

// UsageService exposes the tenant monthly quota usage.
type UsageService struct{ client *Client }

func (s *UsageService) Get(ctx context.Context) (*MonthlyUsage, error) {
	response, err := s.client.raw.GetUsage(ctx)
	if err != nil {
		return nil, err
	}
	result, ok := response.(*api.MonthlyUsage)
	if !ok {
		return nil, unexpectedResponse("GET /v1/usage", response)
	}
	converted, err := convertGenerated[MonthlyUsage](result)
	return &converted, err
}

func unexpectedResponse(operation string, response any) error {
	return fmt.Errorf("viapost: unexpected %s response %T", operation, response)
}
