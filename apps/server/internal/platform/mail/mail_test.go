package mail_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"strings"
	"testing"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/mail"
)

func TestLogMailerWritesMailAtInfo(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	m := mail.New(mail.Config{}, logger) // no SMTP host: log mailer

	err := m.Send(context.Background(), app.Mail{To: "a@example.com", Subject: "ยืนยัน", Text: "สวัสดี"})
	if err != nil {
		t.Fatal(err)
	}

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("log output %q is not one JSON record: %v", buf.String(), err)
	}
	if rec["level"] != "INFO" {
		t.Fatalf("level = %v, want INFO", rec["level"])
	}
	if rec["to"] != "a@example.com" || rec["subject"] != "ยืนยัน" || rec["text"] != "สวัสดี" {
		t.Fatalf("record = %v, want to, subject and text of the mail", rec)
	}
}

func TestSMTPMailerRejectsBadHeadersBeforeDialing(t *testing.T) {
	m := mail.New(mail.Config{Host: "127.0.0.1", Port: 1, From: "no-reply@example.com"}, slog.New(slog.DiscardHandler))
	cases := map[string]app.Mail{
		"newline in subject": {To: "a@example.com", Subject: "hi\r\nBcc: x@evil.test", Text: "body"},
		"newline in to":      {To: "a@example.com\r\nBcc: x@evil.test", Subject: "hi", Text: "body"},
		"not an address":     {To: "not-an-address", Subject: "hi", Text: "body"},
	}
	for name, msg := range cases {
		t.Run(name, func(t *testing.T) {
			err := m.Send(context.Background(), msg)
			if err == nil {
				t.Fatal("expected error")
			}
			// A dial failure would be a *net.OpError; validation must fail first.
			var opErr *net.OpError
			if errors.As(err, &opErr) {
				t.Fatalf("err = %v, want a validation error before any connection", err)
			}
		})
	}
}

func TestSMTPMailerNeedsFromAddress(t *testing.T) {
	m := mail.New(mail.Config{Host: "127.0.0.1", Port: 1}, slog.New(slog.DiscardHandler))
	err := m.Send(context.Background(), app.Mail{To: "a@example.com", Subject: "hi", Text: "body"})
	if err == nil || !strings.Contains(err.Error(), "from") {
		t.Fatalf("err = %v, want one naming the From address", err)
	}
}
