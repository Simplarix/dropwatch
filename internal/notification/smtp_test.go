package notification

import (
	"testing"
	"time"
)

func TestSMTPRejectsPlaintextRemoteServer(t *testing.T) {
	t.Parallel()
	_, err := NewSMTP(SMTPConfig{
		Host: "smtp.example.com", Port: 587, From: "from@example.com",
		To: []string{"to@example.com"}, Timeout: time.Second,
	})
	if err == nil {
		t.Fatal("expected plaintext remote SMTP configuration to be rejected")
	}
}

func TestSMTPAllowsPlaintextLoopbackForTests(t *testing.T) {
	t.Parallel()
	_, err := NewSMTP(SMTPConfig{
		Host: "127.0.0.1", Port: 1025, From: "from@example.com",
		To: []string{"to@example.com"}, Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("loopback SMTP should be allowed without TLS: %v", err)
	}
}
