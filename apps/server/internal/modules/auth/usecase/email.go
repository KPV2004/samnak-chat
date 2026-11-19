package usecase

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth/domain"
)

// EmailJobKind is the job that sends one auth email (AUTH-09). Its args are an
// EmailJob.
const EmailJobKind = "auth.send_email"

// EmailTokenLifetime is how long an emailed link works. A link works once.
const EmailTokenLifetime = time.Hour

// EmailJob is the args of EmailJobKind. The job carries no token: the handler
// makes one, so no usable link waits in the job table.
type EmailJob struct {
	UserID  string              `json:"user_id"`
	Purpose domain.EmailPurpose `json:"purpose"`
}

// ForgotInput is a request for a reset link. IP is "" when unknown.
type ForgotInput struct {
	Email string
	IP    string
}

// emailPaths are the pages of the web app that the links open.
var emailPaths = map[domain.EmailPurpose]string{
	domain.EmailPurposeVerify: "/verify-email",
	domain.EmailPurposeReset:  "/reset-password",
}

// mailText is the subject and the body of a mail. The body is a format string
// taking the user's name and the link, in that order.
type mailText struct{ subject, body string }

var mailTexts = map[domain.EmailPurpose]map[string]mailText{
	domain.EmailPurposeVerify: {
		domain.LocaleThai: {
			subject: "ยืนยันอีเมลของคุณ",
			body:    "สวัสดี %[1]s\n\nกรุณายืนยันอีเมลของคุณโดยเปิดลิงก์ด้านล่าง ลิงก์ใช้ได้ 1 ชั่วโมง และใช้ได้เพียงครั้งเดียว\n\n%[2]s\n\nหากคุณไม่ได้สมัครบัญชี ไม่ต้องทำอะไร",
		},
		domain.LocaleEnglish: {
			subject: "Verify your email",
			body:    "Hello %[1]s,\n\nPlease verify your email by opening the link below. The link works for one hour and only once.\n\n%[2]s\n\nIf you did not sign up, you can ignore this email.",
		},
	},
	domain.EmailPurposeReset: {
		domain.LocaleThai: {
			subject: "รีเซ็ตรหัสผ่านของคุณ",
			body:    "สวัสดี %[1]s\n\nเราได้รับคำขอรีเซ็ตรหัสผ่านของบัญชีนี้ เปิดลิงก์ด้านล่างเพื่อตั้งรหัสผ่านใหม่ ลิงก์ใช้ได้ 1 ชั่วโมง และใช้ได้เพียงครั้งเดียว\n\n%[2]s\n\nหากคุณไม่ได้ขอ ไม่ต้องทำอะไร รหัสผ่านของคุณยังไม่เปลี่ยน",
		},
		domain.LocaleEnglish: {
			subject: "Reset your password",
			body:    "Hello %[1]s,\n\nWe received a request to reset the password of this account. Open the link below to choose a new password. The link works for one hour and only once.\n\n%[2]s\n\nIf you did not ask for this, ignore this email. Your password has not changed.",
		},
	},
}

// renderMail returns the subject and text of a mail in locale. Thai is used
// for a locale without text.
func renderMail(locale string, p domain.EmailPurpose, name, link string) (subject, text string) {
	t, ok := mailTexts[p][locale]
	if !ok {
		t = mailTexts[p][domain.LocaleThai]
	}
	return t.subject, fmt.Sprintf(t.body, name, link)
}

// enqueueEmail queues the mail of purpose for userID. Inside a transaction the
// job exists only if the transaction commits. Without a job queue no mail is
// queued.
func (s *Service) enqueueEmail(ctx context.Context, userID string, p domain.EmailPurpose) error {
	if s.jobs == nil {
		return nil
	}
	return s.jobs.Enqueue(ctx, EmailJobKind, EmailJob{UserID: userID, Purpose: p}, app.EnqueueOptions{})
}

// SendEmail is the handler of EmailJobKind (AUTH-09). It makes a token, stores
// its hash and sends the link. A job can run twice, and then each run makes
// its own token; each one works alone, so a retry is harmless. Without a
// Mailer it logs and does nothing, so no token is made for a mail never sent.
func (s *Service) SendEmail(ctx context.Context, args json.RawMessage) error {
	var job EmailJob
	if err := json.Unmarshal(args, &job); err != nil {
		return fmt.Errorf("send email: args: %w", err)
	}
	if _, ok := emailPaths[job.Purpose]; !ok {
		return fmt.Errorf("send email: unknown purpose %q", job.Purpose)
	}
	if s.mailer == nil {
		s.log.InfoContext(ctx, "auth: no mailer configured, email skipped", "purpose", job.Purpose, "user_id", job.UserID)
		return nil
	}
	u, err := s.repo.UserByID(ctx, job.UserID)
	if notFound(err) {
		// The account was deleted after the job was queued.
		return nil
	}
	if err != nil {
		return err
	}
	token, err := newToken()
	if err != nil {
		return err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("send email: new id: %w", err)
	}
	if err := s.repo.CreateEmailToken(ctx, domain.EmailToken{
		ID: id.String(), UserID: u.ID, Purpose: job.Purpose, TokenHash: hashToken(token),
		ExpiresAt: s.now().Add(EmailTokenLifetime),
	}); err != nil {
		return err
	}
	link := s.webURL + emailPaths[job.Purpose] + "?token=" + url.QueryEscape(token)
	subject, text := renderMail(u.Locale, job.Purpose, u.DisplayName, link)
	return s.mailer.Send(ctx, app.Mail{To: u.Email, Subject: subject, Text: text})
}

// VerifyEmail confirms the address of the account the link was sent to
// (AUTH-09).
func (s *Service) VerifyEmail(ctx context.Context, token string) error {
	return s.tx.Do(ctx, func(ctx context.Context) error {
		t, err := s.consumeEmailToken(ctx, token, domain.EmailPurposeVerify)
		if err != nil {
			return err
		}
		return s.repo.MarkEmailVerified(ctx, t.UserID, s.now())
	})
}

// ResendVerification sends the verification mail again to a signed-in user
// whose address is not verified yet (AUTH-09). An account that is verified
// gets nothing. Each user is limited like a failed login (AUTH-06).
func (s *Service) ResendVerification(ctx context.Context, userID string) error {
	u, err := s.repo.UserByID(ctx, userID)
	if err != nil {
		return err
	}
	if u.EmailVerifiedAt != nil {
		return nil
	}
	ok, err := s.allow(ctx, "resend:user:"+userID)
	if err != nil {
		return err
	}
	if !ok {
		return errResendLimited
	}
	return s.enqueueEmail(ctx, userID, domain.EmailPurposeVerify)
}

// RequestPasswordReset queues a reset link when the email belongs to an
// account (AUTH-09). It says nothing about the account: the caller answers 204
// in every case. A malformed address sends nothing. Every well-formed request
// counts against its email and its IP, whether or not the account exists, and
// a request over the limit sends nothing.
func (s *Service) RequestPasswordReset(ctx context.Context, in ForgotInput) error {
	email, err := domain.NormalizeEmail(in.Email)
	if err != nil {
		return nil
	}
	keys := []string{"forgot:email:" + email}
	if in.IP != "" {
		keys = append(keys, "forgot:ip:"+in.IP)
	}
	ok, err := s.allow(ctx, keys...)
	if err != nil || !ok {
		return err
	}
	u, err := s.repo.UserByEmail(ctx, email)
	if notFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.enqueueEmail(ctx, u.ID, domain.EmailPurposeReset)
}

// ResetPassword sets a new password with a reset link and ends every session
// of the account: its refresh tokens and its devices (AUTH-03, AUTH-04, AUTH-08).
// The link is spent only when the password is accepted.
func (s *Service) ResetPassword(ctx context.Context, token, password string) error {
	if err := domain.ValidatePassword(password); err != nil {
		return invalid(err)
	}
	hash, err := domain.HashPassword(password)
	if err != nil {
		return err
	}
	var (
		userID  string
		revoked bool
	)
	err = s.tx.Do(ctx, func(ctx context.Context) error {
		now := s.now()
		t, err := s.consumeEmailToken(ctx, token, domain.EmailPurposeReset)
		if err != nil {
			return err
		}
		active, err := s.repo.ActiveDevices(ctx, t.UserID)
		if err != nil {
			return err
		}
		if err := s.repo.UpdatePassword(ctx, t.UserID, hash, now); err != nil {
			return err
		}
		if err := s.repo.RevokeUserRefreshTokens(ctx, t.UserID, now); err != nil {
			return err
		}
		if err := s.repo.RevokeUserDevices(ctx, t.UserID, now); err != nil {
			return err
		}
		userID, revoked = t.UserID, len(active) > 0
		return nil
	})
	if err != nil {
		return err
	}
	if revoked {
		s.events.DeviceRevoked(ctx, userID, "")
	}
	return nil
}

// consumeEmailToken spends the token of purpose and returns it. An unknown,
// wrong-purpose, used or expired token all get the same answer. The caller's
// transaction holds the row lock, so two uses cannot both succeed.
func (s *Service) consumeEmailToken(ctx context.Context, token string, p domain.EmailPurpose) (domain.EmailToken, error) {
	t, err := s.repo.EmailTokenForUpdate(ctx, hashToken(token), p)
	if notFound(err) {
		return domain.EmailToken{}, errBadEmailToken
	}
	if err != nil {
		return domain.EmailToken{}, err
	}
	now := s.now()
	if t.UsedAt != nil || !now.Before(t.ExpiresAt) {
		return domain.EmailToken{}, errBadEmailToken
	}
	if err := s.repo.MarkEmailTokenUsed(ctx, t.ID, now); err != nil {
		return domain.EmailToken{}, err
	}
	return t, nil
}

// allow counts one request against each key, unless a key is over its limit.
// It uses the login limiter, so each request is one failure of its key and the
// limit and window are the login ones (AUTH-06). A blocked request counts
// nothing and reports false.
func (s *Service) allow(ctx context.Context, keys ...string) (bool, error) {
	for _, k := range keys {
		blocked, err := s.limiter.Blocked(ctx, k)
		if err != nil {
			return false, fmt.Errorf("limiter: %w", err)
		}
		if blocked {
			return false, nil
		}
	}
	for _, k := range keys {
		if err := s.limiter.Fail(ctx, k); err != nil {
			return false, fmt.Errorf("limiter: %w", err)
		}
	}
	return true, nil
}

// newToken returns 32 random bytes, base64url encoded (AUTH-09).
func newToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("email token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
