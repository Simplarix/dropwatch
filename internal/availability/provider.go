package availability

import (
	"context"
	"time"

	"github.com/Simplarix/dropwatch/internal/rdap"
)

type AvailabilityResult struct {
	Source            string
	Available         bool
	Registered        bool
	Authoritative     bool
	RDAPStatuses      []string
	Registrar         string
	RegisteredAt      *time.Time
	ExpiresAt         *time.Time
	ExternalUpdatedAt *time.Time
	Nameservers       []string
	HTTPStatus        int
	RawResponse       string
}

// AvailabilityProvider is the stable extension point for registrar adapters.
type AvailabilityProvider interface {
	Name() string
	Check(ctx context.Context, domain string) (AvailabilityResult, error)
}

type RDAPProvider struct {
	client *rdap.Client
}

func NewRDAPProvider(client *rdap.Client) *RDAPProvider {
	return &RDAPProvider{client: client}
}

func (p *RDAPProvider) Name() string { return "rdap" }

func (p *RDAPProvider) Check(ctx context.Context, name string) (AvailabilityResult, error) {
	result, err := p.client.Lookup(ctx, name)
	if err != nil {
		return AvailabilityResult{}, err
	}
	return AvailabilityResult{
		Source:            result.Source,
		Available:         !result.Found,
		Registered:        result.Found,
		Authoritative:     result.Authoritative,
		RDAPStatuses:      result.Statuses,
		Registrar:         result.Registrar,
		RegisteredAt:      result.RegisteredAt,
		ExpiresAt:         result.ExpiresAt,
		ExternalUpdatedAt: result.UpdatedAt,
		Nameservers:       result.Nameservers,
		HTTPStatus:        result.HTTPStatus,
		RawResponse:       result.RawResponse,
	}, nil
}

type MockProvider struct {
	ProviderName string
	Result       AvailabilityResult
	Err          error
}

func (m MockProvider) Name() string {
	if m.ProviderName == "" {
		return "mock"
	}
	return m.ProviderName
}

func (m MockProvider) Check(context.Context, string) (AvailabilityResult, error) {
	return m.Result, m.Err
}
