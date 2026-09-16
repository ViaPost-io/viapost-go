package viapost

import (
	"errors"
	"net"
	"net/netip"
	"net/url"
	"strings"

	"github.com/google/uuid"
)

const (
	// MaxVariables is the maximum number of template variables accepted by one send.
	MaxVariables = 100
	// MaxAttachments is the maximum number of attachments accepted by one send.
	MaxAttachments = 10
	// MaxIdempotencyKeyLength is the maximum idempotency key length in Unicode characters.
	MaxIdempotencyKeyLength = 255
)

var (
	// ErrTooManyVariables indicates that SendRequest.Variables exceeds MaxVariables.
	ErrTooManyVariables = errors.New("viapost: variables cannot contain more than 100 properties")
	// ErrTooManyAttachments indicates that SendRequest.Attachments exceeds MaxAttachments.
	ErrTooManyAttachments = errors.New("viapost: attachments cannot contain more than 10 items")
	// ErrInvalidAttachment indicates an attachment without a filename.
	ErrInvalidAttachment = errors.New("viapost: every attachment must have a non-empty filename")
	// ErrInvalidIdempotencyKey indicates an empty or oversized explicit idempotency key.
	ErrInvalidIdempotencyKey = errors.New("viapost: idempotency key must contain between 1 and 255 characters")
	// ErrInvalidStream indicates an unsupported email delivery stream.
	ErrInvalidStream = errors.New("viapost: stream must be transactional or marketing")
	// ErrInvalidTemplateID indicates that a template identifier is not a UUID.
	ErrInvalidTemplateID = errors.New("viapost: template ID must be a valid UUID")
	// ErrInvalidMessagePeriod indicates an unsupported message list period.
	ErrInvalidMessagePeriod = errors.New("viapost: message period must be 24h, 7d, 14d, or 30d")
	// ErrInvalidAutomationStatus indicates an unsupported automation list status.
	ErrInvalidAutomationStatus = errors.New("viapost: automation status must be disabled, enabled, or archived")
	// ErrInvalidWebhookURL indicates that a webhook URL does not satisfy the public HTTPS contract.
	ErrInvalidWebhookURL = errors.New("viapost: webhook URL must be an absolute HTTPS URL without credentials or fragment")
)

func validateSendRequest(request SendRequest, cfg sendConfig) error {
	if request.Stream != "" && request.Stream != StreamTransactional && request.Stream != StreamMarketing {
		return ErrInvalidStream
	}
	if request.TemplateID != nil {
		if _, err := uuid.Parse(*request.TemplateID); err != nil {
			return ErrInvalidTemplateID
		}
	}
	if len(request.Variables) > MaxVariables {
		return ErrTooManyVariables
	}
	if len(request.Attachments) > MaxAttachments {
		return ErrTooManyAttachments
	}
	for _, attachment := range request.Attachments {
		if attachment.Filename == "" {
			return ErrInvalidAttachment
		}
	}
	if cfg.idempotencyKeySet && !isVisibleASCII(cfg.idempotencyKey, MaxIdempotencyKeyLength) {
		return ErrInvalidIdempotencyKey
	}
	return nil
}

func parseWebhookURL(value string) (*url.URL, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
		return nil, ErrInvalidWebhookURL
	}
	host := strings.TrimSuffix(parsed.Hostname(), ".")
	if ip, err := netip.ParseAddr(host); err == nil {
		ip = ip.Unmap()
		if !ip.IsGlobalUnicast() || ip.IsPrivate() || isSpecialPurposeIP(ip) {
			return nil, ErrInvalidWebhookURL
		}
	} else if looksLikeNumericHost(host) || strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return nil, ErrInvalidWebhookURL
	}
	return parsed, nil
}

var specialPurposeNetworks = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("2001:db8::/32"),
}

func isSpecialPurposeIP(ip netip.Addr) bool {
	for _, network := range specialPurposeNetworks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func isVisibleASCII(value string, maximum int) bool {
	if len(value) == 0 || len(value) > maximum {
		return false
	}
	for index := 0; index < len(value); index++ {
		if value[index] < 0x21 || value[index] > 0x7e {
			return false
		}
	}
	return true
}

func looksLikeNumericHost(host string) bool {
	if strings.HasPrefix(strings.ToLower(host), "0x") {
		return true
	}
	for _, character := range host {
		if (character < '0' || character > '9') && character != '.' {
			return false
		}
	}
	return net.ParseIP(host) == nil
}
