package domain

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

// Preferences is one user's notification settings in one workspace (NOTI-07).
// A user with no saved settings has Defaults: every kind on, no channel muted,
// no do-not-disturb window.
type Preferences struct {
	UserID      string
	WorkspaceID string
	// DisabledKinds lists notification kinds the user does not want, e.g.
	// "chat.dm". Notices of these kinds are dropped before they are stored.
	DisabledKinds []string
	// MutedChannelIDs lists chat channels whose notices are dropped.
	MutedChannelIDs []string
	// DNDStart and DNDEnd are "HH:MM" in the user's time zone. Both are empty
	// when there is no window. A start after the end wraps past midnight.
	DNDStart  string
	DNDEnd    string
	UpdatedAt time.Time
}

// Defaults are the settings of a user who never saved any.
func Defaults(workspaceID, userID string) Preferences {
	return Preferences{
		UserID: userID, WorkspaceID: workspaceID,
		DisabledKinds: []string{}, MutedChannelIDs: []string{},
	}
}

var (
	// kindRe is "<module>.<name>" in lower case, the shape of every kind.
	kindRe = regexp.MustCompile(`^[a-z][a-z0-9_]*\.[a-z][a-z0-9_]*$`)
	// clockRe is a 24-hour "HH:MM" time.
	clockRe = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)
)

// Normalize checks a settings change and returns it in canonical form: kinds
// and channel IDs sorted without duplicates, and a do-not-disturb window that
// is either complete or absent. Errors wrap app.ErrInvalid.
//
// Kinds are checked for shape only: notify does not know which kinds rules
// will produce, so a kind no rule uses is stored and simply never matches.
func (p Preferences) Normalize() (Preferences, error) {
	out := p
	kinds := make([]string, 0, len(p.DisabledKinds))
	for _, k := range p.DisabledKinds {
		if !kindRe.MatchString(k) {
			return Preferences{}, fmt.Errorf("kind %q must look like module.name: %w", k, app.ErrInvalid)
		}
		kinds = append(kinds, k)
	}
	slices.Sort(kinds)
	out.DisabledKinds = slices.Compact(kinds)

	channels := make([]string, 0, len(p.MutedChannelIDs))
	for _, c := range p.MutedChannelIDs {
		u, err := uuid.Parse(c)
		if err != nil {
			return Preferences{}, fmt.Errorf("channel %q is not an ID: %w", c, app.ErrInvalid)
		}
		channels = append(channels, u.String())
	}
	slices.Sort(channels)
	out.MutedChannelIDs = slices.Compact(channels)

	if (p.DNDStart == "") != (p.DNDEnd == "") {
		return Preferences{}, fmt.Errorf("dnd_start and dnd_end must both be set or both be empty: %w", app.ErrInvalid)
	}
	if p.DNDStart != "" {
		start, err := clockMinutes(p.DNDStart)
		if err != nil {
			return Preferences{}, fmt.Errorf("dnd_start: %w", err)
		}
		end, err := clockMinutes(p.DNDEnd)
		if err != nil {
			return Preferences{}, fmt.Errorf("dnd_end: %w", err)
		}
		if start == end {
			return Preferences{}, fmt.Errorf("the do-not-disturb window is empty: %w", app.ErrInvalid)
		}
	}
	return out, nil
}

// clockMinutes parses "HH:MM" into minutes after midnight.
func clockMinutes(s string) (int, error) {
	if !clockRe.MatchString(s) {
		return 0, fmt.Errorf("%q is not HH:MM: %w", s, app.ErrInvalid)
	}
	h, _ := strconv.Atoi(s[:2])
	m, _ := strconv.Atoi(s[3:])
	return h*60 + m, nil
}

// KindDisabled reports whether the user turned this kind off.
func (p Preferences) KindDisabled(kind string) bool {
	return slices.Contains(p.DisabledKinds, kind)
}

// ChannelMuted reports whether the user muted this channel. A notice that is
// not about a channel is never muted.
func (p Preferences) ChannelMuted(channelID string) bool {
	return channelID != "" && slices.Contains(p.MutedChannelIDs, channelID)
}

// InWindow reports whether at falls inside the do-not-disturb window read in
// loc, the user's time zone. A nil loc means UTC. The start is inclusive and
// the end exclusive; a window that starts after it ends wraps past midnight.
func (p Preferences) InWindow(at time.Time, loc *time.Location) bool {
	if p.DNDStart == "" || p.DNDEnd == "" {
		return false
	}
	start, err := clockMinutes(p.DNDStart)
	if err != nil {
		return false
	}
	end, err := clockMinutes(p.DNDEnd)
	if err != nil {
		return false
	}
	if loc == nil {
		loc = time.UTC
	}
	local := at.In(loc)
	now := local.Hour()*60 + local.Minute()
	if start < end {
		return start <= now && now < end
	}
	return now >= start || now < end
}
