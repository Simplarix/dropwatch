package notification

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

type SMTPConfig struct {
	Host       string
	Port       int
	Username   string
	Password   string
	From       string
	To         []string
	RequireTLS bool
	Timeout    time.Duration
}

type SMTP struct {
	cfg SMTPConfig
}

func NewSMTP(cfg SMTPConfig) (*SMTP, error) {
	if cfg.Host == "" || cfg.Port < 1 || cfg.Port > 65535 {
		return nil, fmt.Errorf("invalid SMTP host or port")
	}
	if cfg.Timeout <= 0 {
		return nil, fmt.Errorf("SMTP timeout must be positive")
	}
	if !cfg.RequireTLS && !isLoopback(cfg.Host) {
		return nil, fmt.Errorf("SMTP without required TLS is allowed only for loopback servers")
	}
	if _, err := mail.ParseAddress(cfg.From); err != nil {
		return nil, fmt.Errorf("invalid SMTP from address")
	}
	if len(cfg.To) == 0 {
		return nil, fmt.Errorf("SMTP recipients are empty")
	}
	for _, recipient := range cfg.To {
		if _, err := mail.ParseAddress(recipient); err != nil {
			return nil, fmt.Errorf("invalid SMTP recipient")
		}
	}
	return &SMTP{cfg: cfg}, nil
}

func (s *SMTP) Name() string { return "smtp" }

func (s *SMTP) Send(ctx context.Context, notification Notification) error {
	address := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	dialer := net.Dialer{Timeout: s.cfg.Timeout}
	connection, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("connect to SMTP server: %w", err)
	}
	defer connection.Close()
	deadline := time.Now().Add(s.cfg.Timeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	_ = connection.SetDeadline(deadline)
	client, err := smtp.NewClient(connection, s.cfg.Host)
	if err != nil {
		return fmt.Errorf("start SMTP session: %w", err)
	}
	defer client.Close()
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{
			ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12,
		}); err != nil {
			return fmt.Errorf("start SMTP TLS: %w", err)
		}
	} else if s.cfg.RequireTLS {
		return fmt.Errorf("SMTP server does not offer required STARTTLS")
	}
	if s.cfg.Username != "" {
		auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP authentication failed")
		}
	}
	from, _ := mail.ParseAddress(s.cfg.From)
	if err := client.Mail(from.Address); err != nil {
		return fmt.Errorf("set SMTP sender: %w", err)
	}
	for _, rawRecipient := range s.cfg.To {
		recipient, _ := mail.ParseAddress(rawRecipient)
		if err := client.Rcpt(recipient.Address); err != nil {
			return fmt.Errorf("set SMTP recipient: %w", err)
		}
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("start SMTP message: %w", err)
	}
	message := buildEmail(s.cfg.From, s.cfg.To, notification)
	if _, err := io.Copy(writer, strings.NewReader(message)); err != nil {
		_ = writer.Close()
		return fmt.Errorf("write SMTP message: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("finish SMTP message: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("finish SMTP session: %w", err)
	}
	return nil
}

func buildEmail(from string, to []string, notification Notification) string {
	var body strings.Builder
	headers := map[string]string{
		"From":                      cleanHeader(from),
		"To":                        cleanHeader(strings.Join(to, ", ")),
		"Subject":                   cleanHeader(notification.Subject),
		"Date":                      notification.Time.UTC().Format(time.RFC1123Z),
		"MIME-Version":              "1.0",
		"Content-Type":              "text/plain; charset=UTF-8",
		"Content-Transfer-Encoding": "8bit",
	}
	for _, key := range []string{"From", "To", "Subject", "Date", "MIME-Version", "Content-Type", "Content-Transfer-Encoding"} {
		fmt.Fprintf(&body, "%s: %s\r\n", key, headers[key])
	}
	body.WriteString("\r\n")
	body.WriteString(notification.Message)
	body.WriteString("\r\n\r\n")
	fmt.Fprintf(&body, "Domain: %s\r\nPrevious state: %s\r\nCurrent state: %s\r\n", notification.Domain, notification.Previous, notification.Current)
	fmt.Fprintf(&body, "RDAP status: %s\r\nUrgency: %s\r\nTime: %s\r\n",
		strings.Join(notification.RDAPStatus, ", "), notification.Urgency, notification.Time.UTC().Format(time.RFC3339))
	if notification.ExpiresAt != nil {
		fmt.Fprintf(&body, "Expiration: %s\r\n", notification.ExpiresAt.UTC().Format(time.RFC3339))
	}
	if notification.Error != "" {
		fmt.Fprintf(&body, "Error: %s\r\n", safeText(notification.Error))
	}
	return body.String()
}

func cleanHeader(value string) string {
	value = strings.ReplaceAll(value, "\r", "")
	return strings.ReplaceAll(value, "\n", "")
}

// ReadSMTPResponse is intentionally small and exported for protocol-focused tests.
func ReadSMTPResponse(reader io.Reader) (string, error) {
	line, err := bufio.NewReader(reader).ReadString('\n')
	return strings.TrimSpace(line), err
}
