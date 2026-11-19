package usecase

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/notify/domain"
)

// GetPreferences returns the caller's settings in the caller's workspace, or
// the defaults when none were saved (NOTI-07).
func (s *Service) GetPreferences(ctx context.Context, p app.Principal) (domain.Preferences, error) {
	got, err := s.repo.Preferences(ctx, p.WorkspaceID, []string{p.UserID})
	if err != nil {
		return domain.Preferences{}, err
	}
	if pref, ok := got[p.UserID]; ok {
		return pref, nil
	}
	return domain.Defaults(p.WorkspaceID, p.UserID), nil
}

// SetPreferences replaces the caller's settings in the caller's workspace and
// returns what was stored. The owner comes from the Principal, never from in.
// An invalid change is app.ErrInvalid and stores nothing (NOTI-07).
func (s *Service) SetPreferences(ctx context.Context, p app.Principal, in domain.Preferences) (domain.Preferences, error) {
	in.UserID, in.WorkspaceID = p.UserID, p.WorkspaceID
	pref, err := in.Normalize()
	if err != nil {
		return domain.Preferences{}, err
	}
	pref.UpdatedAt = s.clock()
	if err := s.repo.SavePreferences(ctx, pref); err != nil {
		return domain.Preferences{}, fmt.Errorf("notify: save preferences: %w", err)
	}
	return pref, nil
}

// applyPreferences decides, for each rule notice, whether the recipient's
// settings let it through (NOTI-07). A disabled kind or a muted channel drops
// the notice. Inside the do-not-disturb window the notice is kept but marked
// quiet. Each user is judged on their own settings and time zone only. Kept
// notices become storable rows here.
func (s *Service) applyPreferences(ctx context.Context, fresh []app.Notification, at time.Time) ([]pending, error) {
	userIDs := map[string][]string{} // recipients by workspace
	seen := map[[2]string]bool{}
	for _, n := range fresh {
		key := [2]string{n.WorkspaceID, n.UserID}
		if !seen[key] {
			seen[key] = true
			userIDs[n.WorkspaceID] = append(userIDs[n.WorkspaceID], n.UserID)
		}
	}

	prefs := map[[2]string]domain.Preferences{}
	for workspaceID, ids := range userIDs {
		got, err := s.repo.Preferences(ctx, workspaceID, ids)
		if err != nil {
			return nil, fmt.Errorf("notify: load preferences: %w", err)
		}
		for id, p := range got {
			prefs[[2]string{workspaceID, id}] = p
		}
	}

	all := make([]string, 0, len(seen))
	for key := range seen {
		all = append(all, key[1])
	}
	slices.Sort(all)
	zones := s.locations(ctx, slices.Compact(all))

	out := make([]pending, 0, len(fresh))
	for _, n := range fresh {
		p, ok := prefs[[2]string{n.WorkspaceID, n.UserID}]
		if !ok {
			p = domain.Defaults(n.WorkspaceID, n.UserID)
		}
		if p.KindDisabled(n.Kind) || p.ChannelMuted(n.ChannelID) {
			continue
		}
		out = append(out, pending{n: domain.New(n, newID(), at), quiet: p.InWindow(at, zones[n.UserID])})
	}
	return out, nil
}

// locations maps users to their time zones. A user with no zone, an unknown
// zone, or a failed lookup is absent and so reads in UTC: the window still
// applies, and a failed lookup never drops a notice.
func (s *Service) locations(ctx context.Context, userIDs []string) map[string]*time.Location {
	out := map[string]*time.Location{}
	if s.dir == nil || len(userIDs) == 0 {
		return out
	}
	names, err := s.dir.Timezones(ctx, userIDs)
	if err != nil {
		s.log.WarnContext(ctx, "notify: time zones unavailable, using UTC", "error", err)
		return out
	}
	for id, name := range names {
		loc, err := time.LoadLocation(name)
		if err != nil {
			continue
		}
		out[id] = loc
	}
	return out
}
