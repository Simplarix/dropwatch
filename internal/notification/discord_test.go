package notification

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Simplarix/dropwatch/internal/domain"
)

func TestDiscordSend(t *testing.T) {
	t.Parallel()
	var payload string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected request")
		}
		body, _ := io.ReadAll(request.Body)
		payload = string(body)
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	notifier, err := NewDiscord(server.URL, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	err = notifier.Send(context.Background(), Notification{
		Domain: "example.com", Previous: domain.StateUnknown, Current: domain.StateRegistered,
		RDAPStatus: []string{"active"}, Time: time.Now().UTC(), Urgency: "info",
		Subject: "test", Message: "changed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(payload, "example.com") || !strings.Contains(payload, "active") {
		t.Fatalf("unexpected Discord payload: %s", payload)
	}
}

func TestDiscordErrorDoesNotEchoResponse(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, "secret upstream response", http.StatusBadRequest)
	}))
	defer server.Close()
	notifier, _ := NewDiscord(server.URL, time.Second)
	err := notifier.Send(context.Background(), Notification{Time: time.Now().UTC()})
	if err == nil || strings.Contains(err.Error(), "secret upstream") {
		t.Fatalf("unsafe or missing error: %v", err)
	}
}

func TestDiscordTransportErrorDoesNotExposeWebhook(t *testing.T) {
	t.Parallel()
	const webhook = "https://discord.com/api/webhooks/123/super-secret-token"
	notifier, err := NewDiscord(webhook, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	notifier.client.Transport = roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		return nil, &url.Error{Op: "Post", URL: request.URL.String(), Err: errors.New("synthetic failure")}
	})
	err = notifier.Send(context.Background(), Notification{Time: time.Now().UTC()})
	if err == nil {
		t.Fatal("expected transport error")
	}
	if strings.Contains(err.Error(), "super-secret-token") || strings.Contains(err.Error(), webhook) {
		t.Fatalf("transport error exposed webhook credential: %v", err)
	}
}

func TestDiscordRejectsRedirects(t *testing.T) {
	t.Parallel()
	redirected := false
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		redirected = true
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Redirect(writer, nil, target.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	notifier, err := NewDiscord(source.URL, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	err = notifier.Send(context.Background(), Notification{Time: time.Now().UTC()})
	if err == nil {
		t.Fatal("expected redirect response to fail")
	}
	if redirected {
		t.Fatal("Discord client followed a redirect")
	}
}

func TestBuildEmailSanitizesHeaders(t *testing.T) {
	t.Parallel()
	message := buildEmail("from@example.com", []string{"to@example.com"}, Notification{
		Subject: "safe\r\nBcc: victim@example.com", Time: time.Now().UTC(),
	})
	if strings.Contains(message, "\r\nBcc:") {
		t.Fatal("header injection remained in message")
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
