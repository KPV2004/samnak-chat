// Package domain holds the auth entities and their validation rules. It knows
// nothing about SQL, HTTP or JSON.
package domain

import (
	"errors"
	"net/mail"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

// Validation errors. The use case layer maps all of them to app.ErrInvalid.
var (
	ErrInvalidEmail       = errors.New("email is not valid")
	ErrInvalidPassword    = errors.New("password must be 8 to 128 characters")
	ErrInvalidDisplayName = errors.New("display name must be 1 to 100 characters")
	ErrInvalidAvatarURL   = errors.New("avatar url must be an http or https url of at most 2048 characters")
	ErrInvalidLocale      = errors.New("locale must be th or en")
	ErrInvalidTimezone    = errors.New("timezone must be an IANA name such as Asia/Bangkok")
	ErrInvalidPlatform    = errors.New("platform must be web, desktop, ios or android")
)

const (
	maxEmailLen       = 254
	minPasswordLen    = 8
	maxPasswordLen    = 128
	maxDisplayNameLen = 100
	maxAvatarURLLen   = 2048
)

// Locales the UI is translated into (NFR-17).
const (
	LocaleThai    = "th"
	LocaleEnglish = "en"
)

// User is an account. PasswordHash never leaves the module.
type User struct {
	ID           string
	Email        string
	PasswordHash string
	DisplayName  string
	AvatarURL    string
	Locale       string
	Timezone     string
	CreatedAt    time.Time
	// EmailVerifiedAt is nil until the address was confirmed by its link (AUTH-09).
	EmailVerifiedAt *time.Time
}

// NormalizeEmail trims and lower-cases an address and checks that it is a
// plain "local@domain.tld" address (no display name, no spaces).
func NormalizeEmail(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" || len(s) > maxEmailLen {
		return "", ErrInvalidEmail
	}
	addr, err := mail.ParseAddress(s)
	if err != nil || addr.Address != s || addr.Name != "" {
		return "", ErrInvalidEmail
	}
	local, host, ok := strings.Cut(s, "@")
	if !ok || local == "" || !strings.Contains(host, ".") || strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") {
		return "", ErrInvalidEmail
	}
	return s, nil
}

// ValidatePassword checks the length in characters, not bytes, so a Thai
// passphrase counts fairly.
func ValidatePassword(pw string) error {
	if n := utf8.RuneCountInString(pw); n < minPasswordLen || n > maxPasswordLen {
		return ErrInvalidPassword
	}
	return nil
}

// NormalizeDisplayName trims the name and checks its length in characters.
func NormalizeDisplayName(s string) (string, error) {
	s = strings.TrimSpace(s)
	if n := utf8.RuneCountInString(s); n < 1 || n > maxDisplayNameLen {
		return "", ErrInvalidDisplayName
	}
	return s, nil
}

// ValidateAvatarURL accepts the empty string (no avatar) or an absolute
// http(s) URL.
func ValidateAvatarURL(s string) error {
	if s == "" {
		return nil
	}
	u, err := url.Parse(s)
	if len(s) > maxAvatarURLLen || err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ErrInvalidAvatarURL
	}
	return nil
}

func ValidateLocale(s string) error {
	if s != LocaleThai && s != LocaleEnglish {
		return ErrInvalidLocale
	}
	return nil
}

// ValidateTimezone requires a name time.LoadLocation resolves. It also rejects
// "" and "Local", which LoadLocation accepts but which are not IANA names.
func ValidateTimezone(s string) error {
	if s == "" || s == "Local" {
		return ErrInvalidTimezone
	}
	if _, err := time.LoadLocation(s); err != nil {
		return ErrInvalidTimezone
	}
	return nil
}

// Client platforms a device can log in from.
var platforms = map[string]bool{"web": true, "desktop": true, "ios": true, "android": true}

func ValidatePlatform(s string) error {
	if !platforms[s] {
		return ErrInvalidPlatform
	}
	return nil
}
