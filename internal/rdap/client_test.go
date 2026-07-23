package rdap

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	t.Parallel()
	body, err := os.ReadFile(filepath.Join("testdata", "domain.json"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := Parse(body)
	if err != nil {
		t.Fatal(err)
	}
	if result.Registrar != "Example Registrar" {
		t.Fatalf("registrar = %q", result.Registrar)
	}
	if result.ExpiresAt == nil || result.ExpiresAt.Format(time.RFC3339) != "2030-08-13T04:00:00Z" {
		t.Fatalf("unexpected expiration: %v", result.ExpiresAt)
	}
	wantNS := []string{"a.iana-servers.net", "b.iana-servers.net"}
	if !reflect.DeepEqual(result.Nameservers, wantNS) {
		t.Fatalf("nameservers = %#v, want %#v", result.Nameservers, wantNS)
	}
}

func TestParseRejectsUnexpectedObject(t *testing.T) {
	t.Parallel()
	if _, err := Parse([]byte(`{"objectClassName":"entity","handle":"x"}`)); err == nil {
		t.Fatal("expected an error")
	}
}

func TestParseRetryAfter(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if got := parseRetryAfter("120", now); got != 2*time.Minute {
		t.Fatalf("got %s", got)
	}
}

func TestEndpointPrefersSecureService(t *testing.T) {
	t.Parallel()
	client := NewClient(time.Second, "https://data.iana.org/rdap/dns.json", "", time.Hour, 1024, "test")
	client.bootstrap = &bootstrap{Services: []json.RawMessage{
		json.RawMessage(`[["com"],["http://insecure.example/rdap","https://secure.example/rdap"]]`),
	}}
	client.loadedAt = time.Now()
	endpoint, err := client.endpoint(context.Background(), "example.com")
	if err != nil {
		t.Fatal(err)
	}
	if endpoint != "https://secure.example/rdap" {
		t.Fatalf("endpoint = %q", endpoint)
	}
}

func TestValidateRemoteURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		rawURL  string
		wantErr bool
	}{
		{name: "public HTTPS", rawURL: "https://rdap.example/rdap"},
		{name: "HTTP downgrade", rawURL: "http://rdap.example/rdap", wantErr: true},
		{name: "credentials", rawURL: "https://user:password@rdap.example/rdap", wantErr: true},
		{name: "nonstandard port", rawURL: "https://rdap.example:8443/rdap", wantErr: true},
		{name: "loopback name", rawURL: "https://localhost/rdap", wantErr: true},
		{name: "loopback address", rawURL: "https://127.0.0.1/rdap", wantErr: true},
		{name: "private address", rawURL: "https://10.0.0.1/rdap", wantErr: true},
		{name: "metadata address", rawURL: "https://169.254.169.254/latest/meta-data", wantErr: true},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			parsed, err := url.Parse(test.rawURL)
			if err != nil {
				t.Fatal(err)
			}
			err = validateRemoteURL(parsed)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateRemoteURL(%q) error = %v", test.rawURL, err)
			}
		})
	}
}

func TestCheckRedirectRejectsUnsafeTarget(t *testing.T) {
	t.Parallel()
	request := &http.Request{URL: &url.URL{Scheme: "http", Host: "rdap.example"}}
	if err := checkRedirect(request, []*http.Request{{}}); err == nil {
		t.Fatal("expected HTTP redirect to be rejected")
	}
	request.URL = &url.URL{Scheme: "https", Host: "rdap.example"}
	via := make([]*http.Request, defaultMaxRedirects)
	if err := checkRedirect(request, via); err == nil || !strings.Contains(err.Error(), "redirects") {
		t.Fatalf("expected redirect limit error, got %v", err)
	}
}

func TestPublicAddressPolicy(t *testing.T) {
	t.Parallel()
	tests := map[string]bool{
		"8.8.8.8":              true,
		"2001:4860:4860::8888": true,
		"127.0.0.1":            false,
		"10.0.0.1":             false,
		"100.64.0.1":           false,
		"169.254.169.254":      false,
		"192.0.2.1":            false,
		"::1":                  false,
		"fc00::1":              false,
		"2001:db8::1":          false,
	}
	for raw, expected := range tests {
		raw, expected := raw, expected
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			if got := isPublicRDAPAddress(netip.MustParseAddr(raw)); got != expected {
				t.Fatalf("isPublicRDAPAddress(%s) = %t, want %t", raw, got, expected)
			}
		})
	}
}

func TestPublicDialRejectsLoopbackBeforeConnecting(t *testing.T) {
	t.Parallel()
	dial := publicDialContext(&net.Dialer{Timeout: 100 * time.Millisecond})
	if _, err := dial(context.Background(), "tcp", "127.0.0.1:443"); err == nil {
		t.Fatal("expected loopback dial to be rejected")
	}
}
