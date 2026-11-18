// Package domain holds the helpdesk model and the rules that hold regardless
// of storage (SRS HD-01..07, HD-09, HD-18): queues, categories, tickets, the
// status table and visibility.
package domain

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

// MaxNameRunes bounds the names of queues and categories, in characters.
const MaxNameRunes = 100

var prefixPattern = regexp.MustCompile(`^[A-Z]{2,6}$`)

// invalid wraps app.ErrInvalid, which httpx answers with 400.
func invalid(format string, args ...any) error {
	return fmt.Errorf(format+": %w", append(args, app.ErrInvalid)...)
}

// validName trims s and checks it is 1 to MaxNameRunes characters.
func validName(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", invalid("name is required")
	}
	if utf8.RuneCountInString(s) > MaxNameRunes {
		return "", invalid("name is longer than %d characters", MaxNameRunes)
	}
	return s, nil
}

// ValidPrefix upper-cases prefix and checks it is 2 to 6 letters A to Z (HD-04).
func ValidPrefix(prefix string) (string, error) {
	p := strings.ToUpper(strings.TrimSpace(prefix))
	if !prefixPattern.MatchString(p) {
		return "", invalid("prefix must be 2 to 6 letters A to Z")
	}
	return p, nil
}
