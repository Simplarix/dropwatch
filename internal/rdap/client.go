package rdap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Simplarix/dropwatch/internal/domain"
)

const defaultMaxRedirects = 3

var blockedRDAPPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/128"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("ff00::/8"),
}

type HTTPError struct {
	StatusCode int
	RetryAfter time.Duration
}

func (e *HTTPError) Error() string {
	if e.StatusCode == http.StatusTooManyRequests {
		return "RDAP server rate limited the request"
	}
	return fmt.Sprintf("RDAP server returned HTTP %d", e.StatusCode)
}

type Result struct {
	Found         bool
	Authoritative bool
	Statuses      []string
	Registrar     string
	RegisteredAt  *time.Time
	ExpiresAt     *time.Time
	UpdatedAt     *time.Time
	Nameservers   []string
	HTTPStatus    int
	RawResponse   string
	Source        string
}

type Client struct {
	http         *http.Client
	bootstrapURL string
	cachePath    string
	cacheTTL     time.Duration
	maxBodyBytes int64
	userAgent    string

	mu        sync.RWMutex
	bootstrap *bootstrap
	loadedAt  time.Time
}

type bootstrap struct {
	Publication string            `json:"publication"`
	Services    []json.RawMessage `json:"services"`
}

func NewClient(timeout time.Duration, bootstrapURL, cachePath string, cacheTTL time.Duration, maxBodyBytes int64, userAgent string) *Client {
	if maxBodyBytes <= 0 {
		maxBodyBytes = 2 << 20
	}
	return &Client{
		http: &http.Client{
			Timeout:       timeout,
			Transport:     newPublicTransport(timeout),
			CheckRedirect: checkRedirect,
		},
		bootstrapURL: bootstrapURL,
		cachePath:    cachePath,
		cacheTTL:     cacheTTL,
		maxBodyBytes: maxBodyBytes,
		userAgent:    userAgent,
	}
}

func (c *Client) Lookup(ctx context.Context, asciiDomain string) (Result, error) {
	endpoint, err := c.endpoint(ctx, asciiDomain)
	if err != nil {
		return Result{}, err
	}
	base, err := url.Parse(endpoint)
	if err != nil || validateRemoteURL(base) != nil {
		return Result{}, fmt.Errorf("bootstrap returned an invalid RDAP endpoint")
	}
	base.Path = strings.TrimSuffix(base.Path, "/") + "/domain/" + url.PathEscape(asciiDomain)
	base.RawQuery = ""
	base.Fragment = ""
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	if err != nil {
		return Result{}, fmt.Errorf("create RDAP request: %w", err)
	}
	req.Header.Set("Accept", "application/rdap+json, application/json")
	req.Header.Set("User-Agent", c.userAgent)
	response, err := c.http.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("RDAP request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return Result{
			Found: false, Authoritative: true, HTTPStatus: response.StatusCode,
			Source: response.Request.URL.Host,
		}, nil
	}
	if response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return Result{}, &HTTPError{
			StatusCode: response.StatusCode,
			RetryAfter: parseRetryAfter(response.Header.Get("Retry-After"), time.Now().UTC()),
		}
	}
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return Result{}, &HTTPError{StatusCode: response.StatusCode}
	}
	if contentType := response.Header.Get("Content-Type"); contentType != "" {
		mediaType, _, parseErr := mime.ParseMediaType(contentType)
		if parseErr != nil || (mediaType != "application/json" && mediaType != "application/rdap+json") {
			return Result{}, fmt.Errorf("RDAP server returned an unexpected content type")
		}
	}
	body, err := readLimited(response.Body, c.maxBodyBytes)
	if err != nil {
		return Result{}, err
	}
	result, err := Parse(body)
	if err != nil {
		return Result{}, err
	}
	result.Found = true
	result.Authoritative = true
	result.HTTPStatus = response.StatusCode
	result.RawResponse = string(body)
	result.Source = response.Request.URL.Host
	return result, nil
}

func Parse(body []byte) (Result, error) {
	var payload struct {
		ObjectClassName string   `json:"objectClassName"`
		LDHName         string   `json:"ldhName"`
		Status          []string `json:"status"`
		Events          []struct {
			Action string `json:"eventAction"`
			Date   string `json:"eventDate"`
		} `json:"events"`
		Nameservers []struct {
			LDHName     string `json:"ldhName"`
			UnicodeName string `json:"unicodeName"`
		} `json:"nameservers"`
		Entities []struct {
			Roles      []string `json:"roles"`
			VCardArray []any    `json:"vcardArray"`
		} `json:"entities"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return Result{}, fmt.Errorf("decode RDAP response: %w", err)
	}
	if payload.ObjectClassName != "" && !strings.EqualFold(payload.ObjectClassName, "domain") {
		return Result{}, fmt.Errorf("RDAP response is not a domain object")
	}
	if payload.LDHName == "" && len(payload.Status) == 0 && len(payload.Events) == 0 {
		return Result{}, fmt.Errorf("RDAP response contains no recognizable domain data")
	}
	result := Result{Found: true, Statuses: domain.NormalizeStatuses(payload.Status)}
	for _, event := range payload.Events {
		date, err := time.Parse(time.RFC3339, event.Date)
		if err != nil {
			continue
		}
		date = date.UTC()
		switch strings.ToLower(event.Action) {
		case "registration":
			result.RegisteredAt = &date
		case "expiration":
			result.ExpiresAt = &date
		case "last changed", "last update of rdap database":
			if result.UpdatedAt == nil || date.After(*result.UpdatedAt) {
				result.UpdatedAt = &date
			}
		}
	}
	for _, nameserver := range payload.Nameservers {
		name := firstNonEmpty(nameserver.LDHName, nameserver.UnicodeName)
		if name != "" {
			result.Nameservers = append(result.Nameservers, strings.ToLower(strings.TrimSuffix(name, ".")))
		}
	}
	slices.Sort(result.Nameservers)
	result.Nameservers = slices.Compact(result.Nameservers)
	for _, entity := range payload.Entities {
		if containsFold(entity.Roles, "registrar") {
			result.Registrar = vcardName(entity.VCardArray)
			if result.Registrar != "" {
				break
			}
		}
	}
	return result, nil
}

func (c *Client) endpoint(ctx context.Context, asciiDomain string) (string, error) {
	parts := strings.Split(asciiDomain, ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("domain has no TLD")
	}
	tld := strings.ToLower(parts[len(parts)-1])
	data, err := c.loadBootstrap(ctx)
	if err != nil {
		return "", err
	}
	for _, rawService := range data.Services {
		var pair []json.RawMessage
		if err := json.Unmarshal(rawService, &pair); err != nil || len(pair) != 2 {
			continue
		}
		var tlds, endpoints []string
		if json.Unmarshal(pair[0], &tlds) != nil || json.Unmarshal(pair[1], &endpoints) != nil {
			continue
		}
		if !containsFold(tlds, tld) {
			continue
		}
		for _, endpoint := range endpoints {
			parsed, parseErr := url.Parse(endpoint)
			if parseErr == nil && validateRemoteURL(parsed) == nil {
				return endpoint, nil
			}
		}
	}
	return "", fmt.Errorf("no secure RDAP service is published for .%s", tld)
}

func (c *Client) loadBootstrap(ctx context.Context) (*bootstrap, error) {
	c.mu.RLock()
	if c.bootstrap != nil && time.Since(c.loadedAt) < c.cacheTTL {
		result := c.bootstrap
		c.mu.RUnlock()
		return result, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.bootstrap != nil && time.Since(c.loadedAt) < c.cacheTTL {
		return c.bootstrap, nil
	}
	cached, cacheTime, cacheErr := c.readCache()
	if cacheErr == nil && time.Since(cacheTime) < c.cacheTTL {
		c.bootstrap, c.loadedAt = cached, cacheTime
		return cached, nil
	}
	fresh, raw, fetchErr := c.fetchBootstrap(ctx)
	if fetchErr == nil {
		c.bootstrap, c.loadedAt = fresh, time.Now()
		if err := c.writeCache(raw); err != nil {
			// A usable in-memory bootstrap is more important than a cache write.
			return fresh, nil
		}
		return fresh, nil
	}
	if cacheErr == nil {
		c.bootstrap, c.loadedAt = cached, cacheTime
		return cached, nil
	}
	return nil, fmt.Errorf("load IANA RDAP bootstrap: %w", fetchErr)
}

func (c *Client) fetchBootstrap(ctx context.Context) (*bootstrap, []byte, error) {
	parsed, err := url.Parse(c.bootstrapURL)
	if err != nil || validateRemoteURL(parsed) != nil {
		return nil, nil, fmt.Errorf("bootstrap URL must be a public HTTPS URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.bootstrapURL, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	response, err := c.http.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return nil, nil, fmt.Errorf("bootstrap server returned HTTP %d", response.StatusCode)
	}
	body, err := readLimited(response.Body, c.maxBodyBytes)
	if err != nil {
		return nil, nil, err
	}
	data, err := parseBootstrap(body)
	return data, body, err
}

func checkRedirect(request *http.Request, via []*http.Request) error {
	if len(via) >= defaultMaxRedirects {
		return fmt.Errorf("stopped after %d redirects", defaultMaxRedirects)
	}
	if err := validateRemoteURL(request.URL); err != nil {
		return fmt.Errorf("refuse unsafe RDAP redirect: %w", err)
	}
	return nil
}

func validateRemoteURL(target *url.URL) error {
	if target == nil || target.Scheme != "https" || target.Hostname() == "" {
		return fmt.Errorf("URL must use HTTPS and include a host")
	}
	if target.User != nil {
		return fmt.Errorf("URL must not contain credentials")
	}
	if port := target.Port(); port != "" && port != "443" {
		return fmt.Errorf("URL must use the default HTTPS port")
	}
	host := strings.TrimSuffix(strings.ToLower(target.Hostname()), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return fmt.Errorf("URL host is not public")
	}
	if address, err := netip.ParseAddr(host); err == nil && !isPublicRDAPAddress(address) {
		return fmt.Errorf("URL address is not public")
	}
	return nil
}

func newPublicTransport(timeout time.Duration) *http.Transport {
	dialer := &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	return &http.Transport{
		Proxy:                  nil,
		DialContext:            publicDialContext(dialer),
		ForceAttemptHTTP2:      true,
		MaxIdleConns:           10,
		MaxIdleConnsPerHost:    2,
		IdleConnTimeout:        90 * time.Second,
		TLSHandshakeTimeout:    timeout,
		ResponseHeaderTimeout:  timeout,
		ExpectContinueTimeout:  time.Second,
		MaxResponseHeaderBytes: 64 << 10,
	}
}

func publicDialContext(dialer *net.Dialer) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("invalid RDAP address")
		}
		resolved, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, fmt.Errorf("resolve RDAP host: %w", err)
		}
		if len(resolved) == 0 {
			return nil, fmt.Errorf("RDAP host resolved to no addresses")
		}
		for _, candidate := range resolved {
			if !isPublicRDAPAddress(candidate) {
				return nil, fmt.Errorf("RDAP host resolved to a non-public address")
			}
		}
		var dialErrors []error
		for _, candidate := range resolved {
			candidate = candidate.Unmap()
			if network == "tcp4" && !candidate.Is4() {
				continue
			}
			if network == "tcp6" && !candidate.Is6() {
				continue
			}
			connection, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(candidate.String(), port))
			if dialErr == nil {
				return connection, nil
			}
			dialErrors = append(dialErrors, dialErr)
		}
		if len(dialErrors) == 0 {
			return nil, fmt.Errorf("RDAP host has no address for %s", network)
		}
		return nil, fmt.Errorf("connect to RDAP host: %w", errors.Join(dialErrors...))
	}
}

func isPublicRDAPAddress(address netip.Addr) bool {
	address = address.Unmap()
	if !address.IsValid() || !address.IsGlobalUnicast() {
		return false
	}
	for _, prefix := range blockedRDAPPrefixes {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}

func parseBootstrap(body []byte) (*bootstrap, error) {
	var data bootstrap
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("decode bootstrap: %w", err)
	}
	if len(data.Services) == 0 {
		return nil, fmt.Errorf("bootstrap contains no services")
	}
	return &data, nil
}

func (c *Client) readCache() (*bootstrap, time.Time, error) {
	body, err := os.ReadFile(c.cachePath)
	if err != nil {
		return nil, time.Time{}, err
	}
	info, err := os.Stat(c.cachePath)
	if err != nil {
		return nil, time.Time{}, err
	}
	data, err := parseBootstrap(body)
	return data, info.ModTime(), err
}

func (c *Client) writeCache(body []byte) error {
	if err := os.MkdirAll(filepath.Dir(c.cachePath), 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(c.cachePath), ".rdap-bootstrap-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer func() { _ = os.Remove(tempName) }()
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(body); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempName, c.cachePath)
}

func readLimited(reader io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("response exceeds %d bytes", limit)
	}
	return body, nil
}

func parseRetryAfter(value string, now time.Time) time.Duration {
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if deadline, err := http.ParseTime(value); err == nil && deadline.After(now) {
		return deadline.Sub(now)
	}
	return 0
}

func vcardName(card []any) string {
	if len(card) < 2 {
		return ""
	}
	properties, ok := card[1].([]any)
	if !ok {
		return ""
	}
	for _, raw := range properties {
		property, ok := raw.([]any)
		if !ok || len(property) < 4 {
			continue
		}
		name, _ := property[0].(string)
		value, _ := property[3].(string)
		if strings.EqualFold(name, "fn") {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func containsFold(values []string, wanted string) bool {
	for _, value := range values {
		if strings.EqualFold(value, wanted) {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func AsHTTPError(err error) (*HTTPError, bool) {
	var target *HTTPError
	return target, errors.As(err, &target)
}
