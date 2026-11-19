package auth_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth"
)

// The subjects the Thai default and the English locale use (AUTH-09).
const (
	verifySubjectTH = "ยืนยันอีเมลของคุณ"
	resetSubjectTH  = "รีเซ็ตรหัสผ่านของคุณ"
)

// tokenFrom returns the token in the link of m.
func tokenFrom(t *testing.T, m app.Mail) string {
	t.Helper()
	_, after, ok := strings.Cut(m.Text, "token=")
	if !ok {
		t.Fatalf("mail has no link: %q", m.Text)
	}
	tok, _, _ := strings.Cut(after, "\n")
	return tok
}

// oneMail returns the only mail in mails.
func oneMail(t *testing.T, mails []app.Mail) app.Mail {
	t.Helper()
	if len(mails) != 1 {
		t.Fatalf("sent %d mails, want 1", len(mails))
	}
	return mails[0]
}

// verifyToken returns the token of the verification mail that register queued.
func (e *env) verifyToken() string {
	e.t.Helper()
	return tokenFrom(e.t, oneMail(e.t, e.deliverMail()))
}

// resetToken asks for a reset link for a and returns its token. Mail queued
// earlier, such as the verification mail of register, is delivered and dropped.
func (e *env) resetToken(a account) string {
	e.t.Helper()
	e.deliverMail()
	if r := e.forgot(a.Email); r.Status != 204 {
		e.t.Fatalf("forgot: %d %s", r.Status, r.Body)
	}
	return tokenFrom(e.t, oneMail(e.t, e.deliverMail()))
}

func (e *env) forgot(email string) resp {
	e.t.Helper()
	return e.do("POST", "/auth/password/forgot", req{Body: map[string]any{"email": email}})
}

func (e *env) verify(token string) resp {
	e.t.Helper()
	return e.do("POST", "/auth/email/verify", req{Body: map[string]any{"token": token}})
}

func (e *env) resend(accessToken string) resp {
	e.t.Helper()
	return e.do("POST", "/auth/email/verify/resend", req{Token: accessToken})
}

func (e *env) reset(token, password string) resp {
	e.t.Helper()
	return e.do("POST", "/auth/password/reset", req{Body: map[string]any{"token": token, "password": password}})
}

// emailVerified reads email_verified from GET /auth/me.
func (e *env) emailVerified(accessToken string) bool {
	e.t.Helper()
	r := e.do("GET", "/auth/me", req{Token: accessToken})
	if r.Status != 200 {
		e.t.Fatalf("me: %d %s", r.Status, r.Body)
	}
	return r.JSON()["email_verified"] == true
}

func TestRegisterQueuesVerificationMail(t *testing.T) {
	e := newEnv(t)
	a := e.register("Alice")
	m := oneMail(t, e.deliverMail())
	if m.To != a.Email || m.Subject != verifySubjectTH {
		t.Errorf("mail to %q with subject %q", m.To, m.Subject)
	}
	if !strings.Contains(m.Text, "Alice") || !strings.Contains(m.Text, "/verify-email?token=") {
		t.Errorf("mail text = %q", m.Text)
	}
}

func TestEmailTokenIsStoredHashedAndLastAnHour(t *testing.T) {
	e := newEnv(t)
	e.register("Alice")
	tok := e.verifyToken()
	ctx := context.Background()

	sum := sha256.Sum256([]byte(tok))
	var expires time.Time
	err := e.pool.QueryRow(ctx, "SELECT expires_at FROM auth_email_tokens WHERE token_hash = $1", sum[:]).Scan(&expires)
	if err != nil {
		t.Fatalf("no row holds the SHA-256 of the token: %v", err)
	}
	if want := e.clock.Now().Add(time.Hour); !expires.Equal(want) {
		t.Errorf("expires_at = %v, want %v", expires, want)
	}

	var n int
	if err := e.pool.QueryRow(ctx, "SELECT count(*) FROM auth_email_tokens WHERE token_hash = $1", []byte(tok)).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Error("the raw token is stored")
	}
}

func TestRegisterIsRolledBackWhenItsMailCannotBeQueued(t *testing.T) {
	e := newEnv(t)
	e.jobs.enqueueErr = errors.New("queue is down")
	email := uniqueEmail()
	r := e.do("POST", "/auth/register", req{Body: map[string]any{
		"email": email, "password": "correct horse", "display_name": "Alice",
	}})
	if r.Status != 500 {
		t.Fatalf("status = %d: %s", r.Status, r.Body)
	}
	var n int
	if err := e.pool.QueryRow(context.Background(), "SELECT count(*) FROM auth_users WHERE email = $1", email).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Error("the account exists without its verification mail")
	}
}

func TestJobSkipsTheMailWhenNoMailerIsConfigured(t *testing.T) {
	e := startEnv(t, false)
	a := e.register("Alice")
	e.deliverMail() // must not fail: the job logs and skips

	var n int
	err := e.pool.QueryRow(context.Background(),
		"SELECT count(*) FROM auth_email_tokens t JOIN auth_users u ON u.id = t.user_id WHERE u.email = $1", a.Email).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("%d tokens created for a mail nobody sent", n)
	}
}

func TestMailLinksAreBuiltFromTheWebURL(t *testing.T) {
	e := newEnv(t, auth.WithWebURL("https://chat.example.test/"))
	e.register("Alice")
	m := oneMail(t, e.deliverMail())
	if !strings.Contains(m.Text, "https://chat.example.test/verify-email?token=") || strings.Contains(m.Text, "//verify-email") {
		t.Errorf("mail text = %q", m.Text)
	}
}

func TestMailIsInTheUsersLocale(t *testing.T) {
	e := newEnv(t)
	a := e.register("Alice")
	e.deliverMail()

	if r := e.forgot(a.Email); r.Status != 204 {
		t.Fatalf("forgot: %d", r.Status)
	}
	if m := oneMail(t, e.deliverMail()); m.Subject != resetSubjectTH {
		t.Errorf("default locale subject = %q", m.Subject)
	}

	s := e.login(a, "laptop")
	if r := e.do("PATCH", "/auth/me", req{Token: s.Access, Body: map[string]any{"locale": "en"}}); r.Status != 200 {
		t.Fatalf("update locale: %d %s", r.Status, r.Body)
	}
	if r := e.forgot(a.Email); r.Status != 204 {
		t.Fatalf("forgot: %d", r.Status)
	}
	m := oneMail(t, e.deliverMail())
	if m.Subject != "Reset your password" || !strings.Contains(m.Text, "Hello Alice") {
		t.Errorf("english mail = %q / %q", m.Subject, m.Text)
	}
}

func TestVerifyEmailMarksTheAccountVerified(t *testing.T) {
	e := newEnv(t)
	a := e.register("Alice")
	tok := e.verifyToken()
	s := e.login(a, "laptop")
	if e.emailVerified(s.Access) {
		t.Fatal("a new account is verified")
	}
	if r := e.verify(tok); r.Status != 204 {
		t.Fatalf("verify: %d %s", r.Status, r.Body)
	}
	if !e.emailVerified(s.Access) {
		t.Error("the account is not verified after the link was used")
	}
}

func TestVerifyEmailLinkWorksWithinAnHour(t *testing.T) {
	e := newEnv(t)
	e.register("Alice")
	tok := e.verifyToken()
	e.clock.Advance(59 * time.Minute)
	if r := e.verify(tok); r.Status != 204 {
		t.Errorf("verify after 59 minutes: %d %s", r.Status, r.Body)
	}
}

func TestVerifyEmailRejectsAnExpiredToken(t *testing.T) {
	e := newEnv(t)
	a := e.register("Alice")
	tok := e.verifyToken()
	e.clock.Advance(time.Hour)
	if r := e.verify(tok); r.Status != 400 || r.String("error", "code") != "invalid" {
		t.Fatalf("verify after an hour: %d %s", r.Status, r.Body)
	}
	s := e.login(a, "laptop")
	if e.emailVerified(s.Access) {
		t.Error("an expired link verified the account")
	}
}

func TestVerifyEmailTokenIsSingleUse(t *testing.T) {
	e := newEnv(t)
	e.register("Alice")
	tok := e.verifyToken()
	if r := e.verify(tok); r.Status != 204 {
		t.Fatalf("first use: %d %s", r.Status, r.Body)
	}
	if r := e.verify(tok); r.Status != 400 {
		t.Errorf("second use: %d %s", r.Status, r.Body)
	}
}

func TestVerifyEmailRejectsAnUnknownToken(t *testing.T) {
	e := newEnv(t)
	if r := e.verify("not-a-real-token"); r.Status != 400 {
		t.Errorf("unknown token: %d %s", r.Status, r.Body)
	}
}

func TestEmailTokensOnlyWorkForTheirPurpose(t *testing.T) {
	e := newEnv(t)
	a := e.register("Alice")
	verifyTok := e.verifyToken()
	resetTok := e.resetToken(a)

	if r := e.verify(resetTok); r.Status != 400 {
		t.Errorf("a reset link verified the email: %d %s", r.Status, r.Body)
	}
	if r := e.reset(verifyTok, "a new password"); r.Status != 400 {
		t.Errorf("a verification link reset the password: %d %s", r.Status, r.Body)
	}

	// Both refused links left everything as it was.
	s := e.login(a, "laptop")
	if e.emailVerified(s.Access) {
		t.Error("the account is verified after a refused reset link")
	}
}

func TestResendVerificationRequiresLogin(t *testing.T) {
	e := newEnv(t)
	if r := e.resend(""); r.Status != 401 {
		t.Errorf("status = %d, want 401", r.Status)
	}
}

func TestResendVerificationSendsAgainUntilVerified(t *testing.T) {
	e := newEnv(t)
	a := e.register("Alice")
	e.deliverMail()
	s := e.login(a, "laptop")

	if r := e.resend(s.Access); r.Status != 204 {
		t.Fatalf("resend: %d %s", r.Status, r.Body)
	}
	m := oneMail(t, e.deliverMail())
	if m.To != a.Email || m.Subject != verifySubjectTH {
		t.Errorf("resent mail to %q with subject %q", m.To, m.Subject)
	}
	if r := e.verify(tokenFrom(t, m)); r.Status != 204 {
		t.Fatalf("verify: %d %s", r.Status, r.Body)
	}

	if r := e.resend(s.Access); r.Status != 204 {
		t.Fatalf("resend after verifying: %d %s", r.Status, r.Body)
	}
	if n := len(e.deliverMail()); n != 0 {
		t.Errorf("a verified account got %d mails", n)
	}
}

func TestResendVerificationIsRateLimitedPerUser(t *testing.T) {
	e := newEnv(t)
	a := e.register("Alice")
	s := e.login(a, "laptop")
	e.deliverMail()

	for i := 1; i <= 5; i++ {
		if r := e.resend(s.Access); r.Status != 204 {
			t.Fatalf("resend %d: %d %s", i, r.Status, r.Body)
		}
	}
	if r := e.resend(s.Access); r.Status != 429 || r.String("error", "code") != "rate_limited" {
		t.Errorf("resend over the limit: %d %s", r.Status, r.Body)
	}
	if n := len(e.deliverMail()); n != 5 {
		t.Errorf("sent %d mails, want the 5 allowed", n)
	}
}

func TestForgotPasswordSendsAResetLink(t *testing.T) {
	e := newEnv(t)
	a := e.register("Alice")
	e.deliverMail()

	if r := e.forgot(strings.ToUpper(a.Email)); r.Status != 204 {
		t.Fatalf("forgot: %d %s", r.Status, r.Body)
	}
	m := oneMail(t, e.deliverMail())
	if m.To != a.Email || m.Subject != resetSubjectTH {
		t.Errorf("mail to %q with subject %q", m.To, m.Subject)
	}
	if !strings.Contains(m.Text, "/reset-password?token=") {
		t.Errorf("mail text = %q", m.Text)
	}
}

// Forgot answers 204 for every input, and sends nothing unless the address
// belongs to an account, so the response does not reveal accounts.
func TestForgotPasswordForAnUnknownOrMalformedEmailSendsNothing(t *testing.T) {
	e := newEnv(t)
	e.deliverMail()
	for _, email := range []string{uniqueEmail(), "not-an-email", ""} {
		if r := e.forgot(email); r.Status != 204 {
			t.Errorf("forgot(%q) = %d %s, want 204", email, r.Status, r.Body)
		}
	}
	if n := e.jobs.pending("auth.send_email"); n != 0 {
		t.Errorf("%d mails queued, want none", n)
	}
}

func TestForgotPasswordIsRateLimitedPerEmail(t *testing.T) {
	e := newEnv(t)
	a := e.register("Alice")
	e.deliverMail()

	for i := 1; i <= 5; i++ {
		if r := e.forgot(a.Email); r.Status != 204 {
			t.Fatalf("forgot %d: %d %s", i, r.Status, r.Body)
		}
	}
	if r := e.forgot(a.Email); r.Status != 204 {
		t.Errorf("forgot over the limit: %d %s, want 204", r.Status, r.Body)
	}
	if n := len(e.deliverMail()); n != 5 {
		t.Errorf("sent %d mails, want the 5 allowed", n)
	}
}

func TestForgotPasswordIsRateLimitedPerIP(t *testing.T) {
	e := newEnv(t)
	a := e.register("Alice")
	e.deliverMail()

	const ip = "198.51.100.7"
	for i := 1; i <= 5; i++ {
		if r := e.do("POST", "/auth/password/forgot", req{IP: ip, Body: map[string]any{"email": uniqueEmail()}}); r.Status != 204 {
			t.Fatalf("forgot %d: %d %s", i, r.Status, r.Body)
		}
	}
	if r := e.do("POST", "/auth/password/forgot", req{IP: ip, Body: map[string]any{"email": a.Email}}); r.Status != 204 {
		t.Errorf("forgot from a limited IP: %d %s, want 204", r.Status, r.Body)
	}
	if n := e.jobs.pending("auth.send_email"); n != 0 {
		t.Errorf("a limited IP got %d mails queued", n)
	}
}

func TestResetPasswordSetsThePasswordAndEndsEverySession(t *testing.T) {
	e := newEnv(t)
	a := e.register("Alice")
	laptop := e.login(a, "laptop")
	phone := e.login(a, "phone")
	tok := e.resetToken(a)

	const newPassword = "a new password"
	if r := e.reset(tok, newPassword); r.Status != 204 {
		t.Fatalf("reset: %d %s", r.Status, r.Body)
	}

	old := e.do("POST", "/auth/login", req{Body: map[string]any{
		"email": a.Email, "password": a.Password, "device_name": "old", "platform": "web",
	}})
	if old.Status != 401 {
		t.Errorf("old password still logs in: %d", old.Status)
	}
	e.login(account{Email: a.Email, Password: newPassword}, "laptop again")

	if r := e.do("POST", "/auth/refresh", req{Body: map[string]any{"refresh_token": laptop.Refresh}}); r.Status != 401 {
		t.Errorf("laptop refresh after reset: %d", r.Status)
	}
	if r := e.do("POST", "/auth/refresh", req{Body: map[string]any{"refresh_token": phone.Refresh}}); r.Status != 401 {
		t.Errorf("phone refresh after reset: %d", r.Status)
	}
	if r := e.do("GET", "/auth/me", req{Token: phone.Access}); r.Status != 401 {
		t.Errorf("phone access token after reset: %d", r.Status)
	}
}

func TestResetPasswordRejectsAnExpiredLink(t *testing.T) {
	e := newEnv(t)
	a := e.register("Alice")
	tok := e.resetToken(a)
	e.clock.Advance(time.Hour)
	if r := e.reset(tok, "a new password"); r.Status != 400 || r.String("error", "code") != "invalid" {
		t.Fatalf("reset after an hour: %d %s", r.Status, r.Body)
	}
	e.login(a, "laptop") // the old password still works
}

func TestResetLinkIsSingleUse(t *testing.T) {
	e := newEnv(t)
	a := e.register("Alice")
	tok := e.resetToken(a)
	if r := e.reset(tok, "a new password"); r.Status != 204 {
		t.Fatalf("first use: %d %s", r.Status, r.Body)
	}
	if r := e.reset(tok, "another password"); r.Status != 400 {
		t.Errorf("second use: %d %s", r.Status, r.Body)
	}
	e.login(account{Email: a.Email, Password: "a new password"}, "laptop")
}

func TestResetPasswordRejectsAShortPasswordWithoutUsingTheLink(t *testing.T) {
	e := newEnv(t)
	a := e.register("Alice")
	tok := e.resetToken(a)
	if r := e.reset(tok, "short"); r.Status != 400 {
		t.Fatalf("short password: %d %s", r.Status, r.Body)
	}
	if r := e.reset(tok, "a new password"); r.Status != 204 {
		t.Errorf("the link was used up by a rejected password: %d %s", r.Status, r.Body)
	}
}
