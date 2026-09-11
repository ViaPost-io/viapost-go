package viapost

import (
	"encoding/json"
	"fmt"
	"time"
)

// Message is a stable representation of an email and its delivery state.
type Message struct {
	ID              string     `json:"id"`
	Status          string     `json:"status"`
	Stream          string     `json:"stream"`
	FromAddress     string     `json:"from_address"`
	ToAddress       string     `json:"to_address"`
	Subject         *string    `json:"subject,omitempty"`
	RecipientDomain string     `json:"recipient_domain"`
	APIKeyID        string     `json:"api_key_id,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	QueuedAt        *time.Time `json:"queued_at,omitempty"`
	SentAt          *time.Time `json:"sent_at,omitempty"`
	DeliveredAt     *time.Time `json:"delivered_at,omitempty"`
	FailedAt        *time.Time `json:"failed_at,omitempty"`
	FirstOpenedAt   *time.Time `json:"first_opened_at,omitempty"`
	FirstClickedAt  *time.Time `json:"first_clicked_at,omitempty"`
	LastError       *string    `json:"last_error,omitempty"`
}

// MessageEvent is one recorded delivery or engagement event.
type MessageEvent struct {
	Type         string    `json:"type"`
	OccurredAt   time.Time `json:"occurred_at"`
	Recipient    string    `json:"recipient,omitempty"`
	SMTPCode     int       `json:"smtp_code,omitempty"`
	EnhancedCode string    `json:"enhanced_code,omitempty"`
	Diagnostic   string    `json:"diagnostic,omitempty"`
	MXHost       string    `json:"mx_host,omitempty"`
	ClickURL     string    `json:"click_url,omitempty"`
}

// MetricsSummary contains aggregate message counters.
type MetricsSummary struct {
	Total      int `json:"total"`
	Delivered  int `json:"delivered"`
	Opened     int `json:"opened"`
	Clicked    int `json:"clicked"`
	Bounced    int `json:"bounced"`
	Complained int `json:"complained"`
}

// MetricsTimeseriesDay contains one day of delivery metrics.
type MetricsTimeseriesDay struct {
	Date       string `json:"date"`
	Delivered  int    `json:"delivered"`
	Open       int    `json:"open"`
	Click      int    `json:"click"`
	SoftBounce int    `json:"soft_bounce"`
	HardBounce int    `json:"hard_bounce"`
	Complaint  int    `json:"complaint"`
}

// DomainMetrics contains aggregate metrics for one sender domain.
type DomainMetrics struct {
	DomainID   string `json:"domain_id"`
	DomainName string `json:"domain_name"`
	Sent       int    `json:"sent"`
	Delivered  int    `json:"delivered"`
	Opened     int    `json:"opened"`
	Clicked    int    `json:"clicked"`
}

// MetricsResponse contains the current and previous metric windows.
type MetricsResponse struct {
	Since      time.Time              `json:"since"`
	Until      time.Time              `json:"until"`
	Current    MetricsSummary         `json:"current"`
	Previous   MetricsSummary         `json:"previous"`
	Timeseries []MetricsTimeseriesDay `json:"timeseries"`
	ByDomain   []DomainMetrics        `json:"by_domain"`
}

// EngagementResult contains delivered, opened, and clicked totals.
type EngagementResult struct {
	Since     time.Time `json:"since"`
	Delivered int       `json:"delivered"`
	Opened    int       `json:"opened"`
	Clicked   int       `json:"clicked"`
}

// MessageTimeseriesDay contains message state counters for one day.
type MessageTimeseriesDay struct {
	Date       string `json:"date"`
	Queued     int    `json:"queued"`
	Processing int    `json:"processing"`
	Sent       int    `json:"sent"`
	Delivered  int    `json:"delivered"`
	Deferred   int    `json:"deferred"`
	Bounced    int    `json:"bounced"`
	Failed     int    `json:"failed"`
	Rejected   int    `json:"rejected"`
	Complained int    `json:"complained"`
}

// TimeseriesResult contains daily message status counters.
type TimeseriesResult struct {
	Since time.Time              `json:"since"`
	Days  []MessageTimeseriesDay `json:"days"`
}

// Domain represents one sender domain.
type Domain struct {
	ID                  string    `json:"id"`
	Name                string    `json:"name"`
	Status              string    `json:"status"`
	SPFVerified         bool      `json:"spf_verified"`
	DKIMVerified        bool      `json:"dkim_verified"`
	DMARCVerified       bool      `json:"dmarc_verified"`
	ReturnPathSubdomain string    `json:"return_path_subdomain"`
	CreatedAt           time.Time `json:"created_at"`
}

// DNSRecord is a DNS record required to verify a sender domain.
type DNSRecord struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Value   string `json:"value"`
	Purpose string `json:"purpose"`
}

// EmailTemplate identifies a versioned email template.
type EmailTemplate struct {
	ID                    string    `json:"id"`
	Name                  string    `json:"name"`
	CurrentDraftVersionID string    `json:"current_draft_version_id,omitempty"`
	PublishedVersionID    string    `json:"published_version_id,omitempty"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

// TemplateVariable declares one template variable.
type TemplateVariable struct {
	Name          string  `json:"name"`
	Type          string  `json:"var_type"`
	FallbackValue *string `json:"fallback_value,omitempty"`
}

// EmailTemplateVersion is one immutable or draft template version.
type EmailTemplateVersion struct {
	ID            string             `json:"id"`
	VersionNumber int                `json:"version_number"`
	Status        string             `json:"status"`
	Subject       string             `json:"subject,omitempty"`
	ContentJSON   json.RawMessage    `json:"content_json"`
	CompiledHTML  string             `json:"compiled_html,omitempty"`
	CompiledText  string             `json:"compiled_text,omitempty"`
	PublishedAt   *time.Time         `json:"published_at,omitempty"`
	Variables     []TemplateVariable `json:"variables"`
	CreatedAt     time.Time          `json:"created_at"`
	UpdatedAt     time.Time          `json:"updated_at"`
}

// CreateTemplateResult contains a new template and its initial draft.
type CreateTemplateResult struct {
	Template EmailTemplate        `json:"template"`
	Draft    EmailTemplateVersion `json:"draft"`
}

// WebhookEndpoint is one outbound webhook subscription.
type WebhookEndpoint struct {
	ID          string    `json:"id"`
	URL         string    `json:"url"`
	EventTypes  []string  `json:"event_types"`
	Enabled     bool      `json:"enabled"`
	MaxAttempts int       `json:"max_attempts"`
	CreatedAt   time.Time `json:"created_at"`
}

// CreateWebhookResult contains the endpoint and its one-time signing secret.
type CreateWebhookResult struct {
	Endpoint WebhookEndpoint `json:"endpoint"`
	Secret   string          `json:"secret"`
}

// Automation represents an automation definition. Graph remains opaque so
// graph evolution does not break SDK consumers.
type Automation struct {
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	Status           string          `json:"status"`
	Graph            json.RawMessage `json:"graph"`
	CurrentVersionID *string         `json:"current_version_id,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

// UsagePeriod identifies a UTC billing period.
type UsagePeriod struct {
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
	Timezone string    `json:"timezone"`
}

// MonthlyUsage is the tenant quota usage for one billing period.
type MonthlyUsage struct {
	Period    UsagePeriod `json:"period"`
	Used      int64       `json:"used"`
	Limit     *int64      `json:"limit"`
	Remaining *int64      `json:"remaining"`
	Unlimited bool        `json:"unlimited"`
}

func convertGenerated[T any](source any) (T, error) {
	var target T
	encoded, err := json.Marshal(source)
	if err != nil {
		return target, fmt.Errorf("viapost: encode generated response: %w", err)
	}
	if err := json.Unmarshal(encoded, &target); err != nil {
		return target, fmt.Errorf("viapost: decode stable response: %w", err)
	}
	return target, nil
}
