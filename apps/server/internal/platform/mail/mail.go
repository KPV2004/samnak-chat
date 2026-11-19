package mail

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"mime/quotedprintable"
	"net"
	netmail "net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

// sendTimeout bounds one message from dial to end of data, unless the
// context has an earlier deadline.
const sendTimeout = 30 * time.Second

// implicitTLSPort is the SMTPS port: the connection is TLS from the first byte.
const implicitTLSPort = 465

// Config describes the SMTP server. An empty Host selects the log mailer.
type Config struct {
	Host     string
	Port     int
	Username string
	Password string
	// From is the sender, for example "Samnak <no-reply@example.com>".
	From string
}

// New returns an SMTP mailer when cfg.Host is set. Otherwise it returns a
// mailer that only logs each message, so development works without a server.
func New(cfg Config, logger *slog.Logger) app.Mailer {
	if cfg.Host == "" {
		return logMailer{log: logger}
	}
	return &smtpMailer{cfg: cfg}
}

type logMailer struct {
	log *slog.Logger
}

func (m logMailer) Send(ctx context.Context, msg app.Mail) error {
	m.log.InfoContext(ctx, "mail not sent: SMTP_HOST is not set",
		"to", msg.To, "subject", msg.Subject, "text", msg.Text)
	return nil
}

type smtpMailer struct {
	cfg Config
}

// Send delivers one plain-text message. Port 465 uses implicit TLS. On other
// ports the mailer upgrades with STARTTLS when the server offers it. Credentials
// go out only over TLS or to localhost, which net/smtp enforces.
func (m *smtpMailer) Send(ctx context.Context, msg app.Mail) error {
	from, err := netmail.ParseAddress(m.cfg.From)
	if err != nil {
		return fmt.Errorf("mail: from address %q: %w", m.cfg.From, err)
	}
	to, err := netmail.ParseAddress(msg.To)
	if err != nil || to.Address != msg.To {
		return fmt.Errorf("mail: invalid recipient %q", msg.To)
	}
	if strings.ContainsAny(msg.Subject, "\r\n") {
		return errors.New("mail: subject must not contain line breaks")
	}
	data := buildMessage(from, to.Address, msg)

	addr := net.JoinHostPort(m.cfg.Host, strconv.Itoa(m.cfg.Port))
	dialer := &net.Dialer{Timeout: sendTimeout}
	var conn net.Conn
	if m.cfg.Port == implicitTLSPort {
		tlsDialer := &tls.Dialer{NetDialer: dialer, Config: &tls.Config{ServerName: m.cfg.Host, MinVersion: tls.VersionTLS12}}
		conn, err = tlsDialer.DialContext(ctx, "tcp", addr)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("mail: dial %s: %w", addr, err)
	}
	deadline := time.Now().Add(sendTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	c, err := smtp.NewClient(conn, m.cfg.Host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("mail: smtp greeting: %w", err)
	}
	defer func() { _ = c.Close() }()

	if m.cfg.Port != implicitTLSPort {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(&tls.Config{ServerName: m.cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
				return fmt.Errorf("mail: starttls: %w", err)
			}
		}
	}
	if m.cfg.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", m.cfg.Username, m.cfg.Password, m.cfg.Host)); err != nil {
			return fmt.Errorf("mail: auth: %w", err)
		}
	}
	if err := c.Mail(from.Address); err != nil {
		return fmt.Errorf("mail: sender rejected: %w", err)
	}
	if err := c.Rcpt(to.Address); err != nil {
		return fmt.Errorf("mail: recipient rejected: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("mail: data: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		_ = w.Close()
		return fmt.Errorf("mail: write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("mail: end of data: %w", err)
	}
	_ = c.Quit()
	return nil
}

// buildMessage returns the RFC 5322 message. The subject is an RFC 2047
// encoded word when it is not ASCII, and the body is quoted-printable UTF-8.
func buildMessage(from *netmail.Address, to string, msg app.Mail) []byte {
	var body bytes.Buffer
	qp := quotedprintable.NewWriter(&body)
	_, _ = qp.Write([]byte(msg.Text))
	_ = qp.Close()

	var b bytes.Buffer
	fmt.Fprintf(&b, "From: %s\r\n", from.String())
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("UTF-8", msg.Subject))
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("Content-Transfer-Encoding: quoted-printable\r\n")
	b.WriteString("\r\n")
	b.Write(body.Bytes())
	return b.Bytes()
}
