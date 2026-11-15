// Package domain holds the workspace module's entities and business rules.
// It knows nothing about SQL, HTTP or other modules.
package domain

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

const maxNameLen = 100

var slugPattern = regexp.MustCompile(`^[a-z0-9-]{3,40}$`)

// Workspace is one organization's space.
type Workspace struct {
	ID        string
	Name      string
	Slug      string
	CreatedAt time.Time
}

// ValidateSlug checks the slug format (WSP-01): 3 to 40 lower-case letters,
// digits or hyphens.
func ValidateSlug(slug string) error {
	if !slugPattern.MatchString(slug) {
		return fmt.Errorf("slug must match %s: %w", slugPattern, app.ErrInvalid)
	}
	return nil
}

// NormalizeName trims the name and checks that it is 1 to 100 characters.
func NormalizeName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxNameLen {
		return "", fmt.Errorf("name must be 1 to %d characters: %w", maxNameLen, app.ErrInvalid)
	}
	return name, nil
}
