package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth/domain"
)

func TestNormalizeEmail(t *testing.T) {
	cases := []struct {
		in, want string
		bad      bool
	}{
		{"  Alice@Example.COM ", "alice@example.com", false},
		{"bob@example.com", "bob@example.com", false},
		{"", "", true},
		{"no-at-sign", "", true},
		{"a@b", "", true},
		{"Alice <alice@example.com>", "", true},
		{"a b@example.com", "", true},
		{"@example.com", "", true},
		{strings.Repeat("a", 250) + "@example.com", "", true},
	}
	for _, c := range cases {
		got, err := domain.NormalizeEmail(c.in)
		if c.bad {
			if !errors.Is(err, domain.ErrInvalidEmail) {
				t.Errorf("NormalizeEmail(%q) err = %v, want ErrInvalidEmail", c.in, err)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("NormalizeEmail(%q) = %q, %v; want %q", c.in, got, err, c.want)
		}
	}
}

func TestValidatePassword(t *testing.T) {
	for _, pw := range []string{"12345678", strings.Repeat("x", 128), "รหัสผ่านยาวพอ"} {
		if err := domain.ValidatePassword(pw); err != nil {
			t.Errorf("ValidatePassword(%q) = %v, want nil", pw, err)
		}
	}
	for _, pw := range []string{"", "1234567", strings.Repeat("x", 129)} {
		if err := domain.ValidatePassword(pw); !errors.Is(err, domain.ErrInvalidPassword) {
			t.Errorf("ValidatePassword(len %d) = %v, want ErrInvalidPassword", len(pw), err)
		}
	}
}

func TestPasswordHashRoundTrip(t *testing.T) {
	hash, err := domain.HashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("hash = %q, want argon2id PHC string with documented parameters", hash)
	}
	if !domain.VerifyPassword(hash, "correct horse") {
		t.Error("correct password rejected")
	}
	if domain.VerifyPassword(hash, "wrong horse") {
		t.Error("wrong password accepted")
	}
	other, _ := domain.HashPassword("correct horse")
	if other == hash {
		t.Error("two hashes of one password are identical: salt is not random")
	}
}

func TestVerifyPasswordRejectsMalformedHash(t *testing.T) {
	for _, h := range []string{"", "plain", "$argon2id$v=19$m=1,t=1,p=1$!!$!!", "$bcrypt$x", "$argon2id$v=19$m=19456,t=2,p=1$c2FsdA"} {
		if domain.VerifyPassword(h, "x") {
			t.Errorf("VerifyPassword(%q) = true", h)
		}
	}
}

func TestLocale(t *testing.T) {
	for _, l := range []string{"th", "en"} {
		if err := domain.ValidateLocale(l); err != nil {
			t.Errorf("ValidateLocale(%q) = %v", l, err)
		}
	}
	for _, l := range []string{"", "fr", "TH"} {
		if err := domain.ValidateLocale(l); !errors.Is(err, domain.ErrInvalidLocale) {
			t.Errorf("ValidateLocale(%q) = %v, want ErrInvalidLocale", l, err)
		}
	}
}

func TestTimezone(t *testing.T) {
	for _, z := range []string{"Asia/Bangkok", "UTC", "America/New_York"} {
		if err := domain.ValidateTimezone(z); err != nil {
			t.Errorf("ValidateTimezone(%q) = %v", z, err)
		}
	}
	// "" and "Local" are accepted by time.LoadLocation but are not IANA names.
	for _, z := range []string{"", "Local", "Mars/Olympus", "../etc/passwd"} {
		if err := domain.ValidateTimezone(z); !errors.Is(err, domain.ErrInvalidTimezone) {
			t.Errorf("ValidateTimezone(%q) = %v, want ErrInvalidTimezone", z, err)
		}
	}
}

func TestDisplayName(t *testing.T) {
	got, err := domain.NormalizeDisplayName("  Alice  ")
	if err != nil || got != "Alice" {
		t.Fatalf("got %q, %v", got, err)
	}
	for _, n := range []string{"", "   ", strings.Repeat("ก", 101)} {
		if _, err := domain.NormalizeDisplayName(n); !errors.Is(err, domain.ErrInvalidDisplayName) {
			t.Errorf("NormalizeDisplayName(len %d) = %v, want ErrInvalidDisplayName", len(n), err)
		}
	}
}

func TestAvatarURL(t *testing.T) {
	for _, u := range []string{"", "https://cdn.example.com/a.png", "http://localhost:9000/a.png"} {
		if err := domain.ValidateAvatarURL(u); err != nil {
			t.Errorf("ValidateAvatarURL(%q) = %v", u, err)
		}
	}
	for _, u := range []string{"javascript:alert(1)", "ftp://x/y", "not a url", "//x/y", "https://" + strings.Repeat("a", 2100)} {
		if err := domain.ValidateAvatarURL(u); !errors.Is(err, domain.ErrInvalidAvatarURL) {
			t.Errorf("ValidateAvatarURL(%q) = %v, want ErrInvalidAvatarURL", u, err)
		}
	}
}

func TestPlatform(t *testing.T) {
	for _, p := range []string{"web", "desktop", "ios", "android"} {
		if err := domain.ValidatePlatform(p); err != nil {
			t.Errorf("ValidatePlatform(%q) = %v", p, err)
		}
	}
	if err := domain.ValidatePlatform("symbian"); !errors.Is(err, domain.ErrInvalidPlatform) {
		t.Errorf("err = %v", err)
	}
}
