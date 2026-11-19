package mail_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/mail"
)

// startMailpit returns an SMTP mailer for a fresh Mailpit container and the
// base URL of its HTTP API.
func startMailpit(t *testing.T) (app.Mailer, string) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	if _, err := os.Stat("/var/run/docker.sock"); err != nil && os.Getenv("DOCKER_HOST") == "" {
		t.Skip("skipping integration test: Docker is not available")
	}
	ctx := context.Background()
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "axllent/mailpit:latest",
			ExposedPorts: []string{"1025/tcp", "8025/tcp"},
			WaitingFor:   wait.ForHTTP("/api/v1/info").WithPort("8025/tcp").WithStartupTimeout(2 * time.Minute),
		},
		Started: true,
	})
	if err != nil {
		t.Skipf("skipping integration test: cannot start mailpit container: %v", err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(c) })

	host, err := c.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	smtpPort, err := c.MappedPort(ctx, "1025/tcp")
	if err != nil {
		t.Fatal(err)
	}
	apiPort, err := c.MappedPort(ctx, "8025/tcp")
	if err != nil {
		t.Fatal(err)
	}

	m := mail.New(mail.Config{
		Host: host,
		Port: int(smtpPort.Num()),
		From: "Samnak <no-reply@samnak.test>",
	}, slog.New(slog.DiscardHandler))
	return m, fmt.Sprintf("http://%s:%s", host, apiPort.Port())
}

type mailpitSummary struct {
	Messages []struct {
		ID      string `json:"ID"`
		Subject string `json:"Subject"`
		To      []struct {
			Address string `json:"Address"`
		} `json:"To"`
	} `json:"messages"`
}

// getJSON decodes a Mailpit API response into v.
func getJSON(t *testing.T, url string, v any) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status %d: %s", url, resp.StatusCode, body)
	}
	if err := json.Unmarshal(body, v); err != nil {
		t.Fatalf("GET %s: %v: %s", url, err, body)
	}
}

// findMessage polls the Mailpit inbox until a message to addr arrives.
func findMessage(t *testing.T, apiBase, addr string) (subject, text string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var list mailpitSummary
		getJSON(t, apiBase+"/api/v1/messages", &list)
		for _, m := range list.Messages {
			for _, to := range m.To {
				if to.Address != addr {
					continue
				}
				var full struct {
					Text string `json:"Text"`
				}
				getJSON(t, apiBase+"/api/v1/message/"+m.ID, &full)
				return m.Subject, full.Text
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("no message to %s in Mailpit", addr)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func TestSMTPMailerDeliversThroughMailpit(t *testing.T) {
	m, apiBase := startMailpit(t)
	subject := "ยืนยันอีเมลของคุณ"
	text := "กดลิงก์เพื่อยืนยัน: http://localhost:5173/verify-email?token=abc123\nSamnak"

	err := m.Send(context.Background(), app.Mail{
		To:      "user@samnak.test",
		Subject: subject,
		Text:    text,
	})
	if err != nil {
		t.Fatal(err)
	}

	gotSubject, gotText := findMessage(t, apiBase, "user@samnak.test")
	if gotSubject != subject {
		t.Fatalf("subject = %q, want %q", gotSubject, subject)
	}
	if !strings.Contains(gotText, "http://localhost:5173/verify-email?token=abc123") || !strings.Contains(gotText, "กดลิงก์เพื่อยืนยัน") {
		t.Fatalf("text = %q, want the sent body", gotText)
	}
}
