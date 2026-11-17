package domain

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

const maxGroupNameLen = 100

// Group is a department or team inside one workspace (WSP-09).
type Group struct {
	ID          string
	WorkspaceID string
	Name        string
	CreatedAt   time.Time
}

// NormalizeGroupName trims the name and checks that it is 1 to 100 characters.
func NormalizeGroupName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxGroupNameLen {
		return "", fmt.Errorf("group name must be 1 to %d characters: %w", maxGroupNameLen, app.ErrInvalid)
	}
	return name, nil
}
