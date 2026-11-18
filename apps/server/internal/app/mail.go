package app

import "context"

// Mail is one plain-text email to one recipient (AUTH-09).
type Mail struct {
	To      string
	Subject string
	Text    string
}

// Mailer sends email. It is implemented by internal/platform/mail: SMTP when
// SMTP_HOST is set, otherwise a logger that writes the mail to the server log
// so development works without a mail server. Send does network I/O, so call
// it from a job handler (Jobs), never inside a request transaction.
type Mailer interface {
	Send(ctx context.Context, m Mail) error
}
