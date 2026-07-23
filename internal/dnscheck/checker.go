package dnscheck

import (
	"context"
	"fmt"
	"net"
	"slices"
	"strings"
	"time"

	"github.com/Simplarix/dropwatch/internal/domain"
	"github.com/miekg/dns"
)

type Checker interface {
	Check(ctx context.Context, asciiDomain string) domain.DNSResult
}

type DNSChecker struct {
	server  string
	timeout time.Duration
}

func New(timeout time.Duration) *DNSChecker {
	server := "1.1.1.1:53"
	if config, err := dns.ClientConfigFromFile("/etc/resolv.conf"); err == nil && len(config.Servers) > 0 {
		server = net.JoinHostPort(config.Servers[0], config.Port)
	}
	return &DNSChecker{server: server, timeout: timeout}
}

func (c *DNSChecker) Check(ctx context.Context, asciiDomain string) domain.DNSResult {
	var result domain.DNSResult
	var errors []string
	for _, query := range []struct {
		recordType uint16
		target     *[]string
	}{
		{dns.TypeNS, &result.NS},
		{dns.TypeSOA, &result.SOA},
		{dns.TypeA, &result.A},
		{dns.TypeAAAA, &result.AAAA},
		{dns.TypeMX, &result.MX},
	} {
		records, err := c.lookup(ctx, asciiDomain, query.recordType)
		if err != nil {
			errors = append(errors, dns.TypeToString[query.recordType]+": "+safeDNSError(err))
			continue
		}
		*query.target = records
	}
	result.Active = len(result.NS)+len(result.SOA)+len(result.A)+len(result.AAAA)+len(result.MX) > 0
	if len(errors) > 0 {
		result.Error = strings.Join(errors, "; ")
	}
	return result
}

func (c *DNSChecker) lookup(ctx context.Context, name string, recordType uint16) ([]string, error) {
	message := new(dns.Msg)
	message.SetQuestion(dns.Fqdn(name), recordType)
	client := &dns.Client{Timeout: c.timeout}
	response, _, err := client.ExchangeContext(ctx, message, c.server)
	if err != nil {
		return nil, err
	}
	if response.Truncated {
		tcpClient := &dns.Client{Net: "tcp", Timeout: c.timeout}
		response, _, err = tcpClient.ExchangeContext(ctx, message, c.server)
		if err != nil {
			return nil, err
		}
	}
	if response.Rcode != dns.RcodeSuccess && response.Rcode != dns.RcodeNameError {
		return nil, fmt.Errorf("DNS response code %s", dns.RcodeToString[response.Rcode])
	}
	var records []string
	for _, answer := range response.Answer {
		switch value := answer.(type) {
		case *dns.NS:
			records = append(records, strings.TrimSuffix(strings.ToLower(value.Ns), "."))
		case *dns.SOA:
			records = append(records, strings.TrimSuffix(strings.ToLower(value.Ns), "."))
		case *dns.A:
			records = append(records, value.A.String())
		case *dns.AAAA:
			records = append(records, value.AAAA.String())
		case *dns.MX:
			records = append(records, fmt.Sprintf("%d %s", value.Preference, strings.TrimSuffix(strings.ToLower(value.Mx), ".")))
		}
	}
	slices.Sort(records)
	return slices.Compact(records), nil
}

func safeDNSError(err error) string {
	value := strings.ReplaceAll(err.Error(), "\n", " ")
	if len(value) > 200 {
		return value[:200]
	}
	return value
}
