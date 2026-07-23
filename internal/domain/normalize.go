package domain

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"golang.org/x/net/idna"
)

type NormalizedName struct {
	Unicode string
	ASCII   string
}

func NormalizeName(input string) (NormalizedName, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return NormalizedName{}, fmt.Errorf("domain is empty")
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return NormalizedName{}, fmt.Errorf("parse domain: %w", err)
	}
	host := parsed.Hostname()
	if host == "" {
		return NormalizedName{}, fmt.Errorf("domain has no hostname")
	}
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if net.ParseIP(host) != nil {
		return NormalizedName{}, fmt.Errorf("IP addresses are not domains")
	}
	ascii, err := idna.Lookup.ToASCII(host)
	if err != nil {
		return NormalizedName{}, fmt.Errorf("normalize IDN: %w", err)
	}
	ascii = strings.ToLower(strings.TrimSuffix(ascii, "."))
	if err := validateASCIIName(ascii); err != nil {
		return NormalizedName{}, err
	}
	unicode, err := idna.Lookup.ToUnicode(ascii)
	if err != nil {
		return NormalizedName{}, fmt.Errorf("decode IDN: %w", err)
	}
	return NormalizedName{Unicode: strings.ToLower(unicode), ASCII: ascii}, nil
}

func validateASCIIName(name string) error {
	if len(name) > 253 {
		return fmt.Errorf("domain exceeds 253 bytes")
	}
	labels := strings.Split(name, ".")
	if len(labels) < 2 {
		return fmt.Errorf("domain must contain a public suffix")
	}
	for _, label := range labels {
		if label == "" || len(label) > 63 {
			return fmt.Errorf("invalid domain label length")
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return fmt.Errorf("domain labels cannot start or end with a hyphen")
		}
		for _, char := range label {
			if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
				return fmt.Errorf("invalid character in domain")
			}
		}
	}
	return nil
}
