package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Discord struct {
	webhookURL string
	client     *http.Client
}

func NewDiscord(webhookURL string, timeout time.Duration) (*Discord, error) {
	parsed, err := url.Parse(webhookURL)
	if err != nil || parsed.Host == "" {
		return nil, fmt.Errorf("invalid Discord webhook URL")
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && isLoopback(parsed.Hostname())) {
		return nil, fmt.Errorf("Discord webhook URL must use HTTPS")
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("Discord timeout must be positive")
	}
	return &Discord{
		webhookURL: webhookURL,
		client: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func (d *Discord) Name() string { return "discord" }

func (d *Discord) Send(ctx context.Context, notification Notification) error {
	color := 0x3498db
	switch notification.Urgency {
	case "warning":
		color = 0xf39c12
	case "critical", "error":
		color = 0xe74c3c
	}
	fields := []map[string]any{
		{"name": "Domain", "value": notification.Domain, "inline": true},
		{"name": "State", "value": fmt.Sprintf("%s → %s", notification.Previous, notification.Current), "inline": true},
		{"name": "Urgency", "value": notification.Urgency, "inline": true},
		{"name": "RDAP status", "value": fallback(strings.Join(notification.RDAPStatus, ", "), "n/a"), "inline": false},
	}
	if notification.ExpiresAt != nil {
		fields = append(fields, map[string]any{
			"name": "Expiration", "value": notification.ExpiresAt.UTC().Format(time.RFC3339), "inline": true,
		})
	}
	if notification.Error != "" {
		fields = append(fields, map[string]any{"name": "Error", "value": safeText(notification.Error), "inline": false})
	}
	payload := map[string]any{
		"username": "DropWatch",
		"embeds": []map[string]any{{
			"title": notification.Subject, "description": notification.Message,
			"color": color, "timestamp": notification.Time.UTC().Format(time.RFC3339),
			"fields": fields,
		}},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode Discord message: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, d.webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create Discord request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := d.client.Do(request)
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return fmt.Errorf("send Discord notification: %w", contextErr)
		}
		// net/http transport errors may contain the complete request URL. A
		// Discord webhook URL is a credential and must never reach logs or
		// persistent notification-attempt records.
		return fmt.Errorf("send Discord notification: transport failure")
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Discord returned HTTP %d", response.StatusCode)
	}
	return nil
}

func isLoopback(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	return host == "localhost" || strings.HasSuffix(host, ".localhost") || net.ParseIP(host).IsLoopback()
}

func safeText(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\n", " "))
	if len(value) > 1000 {
		return value[:1000]
	}
	return value
}

func fallback(value, replacement string) string {
	if value == "" {
		return replacement
	}
	return value
}
