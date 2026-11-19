package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/notify/adapter/postgres/gen"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/notify/domain"
)

// Preferences returns the saved settings of the given users, keyed by user ID.
// A user with no row is absent from the map, not an error (NOTI-07).
func (r *Repo) Preferences(ctx context.Context, workspaceID string, userIDs []string) (map[string]domain.Preferences, error) {
	q, err := r.q(ctx)
	if err != nil {
		return nil, err
	}
	wid, err := id(workspaceID)
	if err != nil {
		return nil, err
	}
	uids := make([]uuid.UUID, 0, len(userIDs))
	for _, u := range userIDs {
		uid, err := id(u)
		if err != nil {
			return nil, err
		}
		uids = append(uids, uid)
	}
	out := map[string]domain.Preferences{}
	if len(uids) == 0 {
		return out, nil
	}
	rows, err := q.GetPreferences(ctx, gen.GetPreferencesParams{WorkspaceID: wid, UserIds: uids})
	if err != nil {
		return nil, wrap(err, "get preferences")
	}
	for _, row := range rows {
		p := toPreferences(row)
		out[p.UserID] = p
	}
	return out, nil
}

// SavePreferences stores p as the user's whole settings, replacing any row.
func (r *Repo) SavePreferences(ctx context.Context, p domain.Preferences) error {
	q, err := r.q(ctx)
	if err != nil {
		return err
	}
	wid, err := id(p.WorkspaceID)
	if err != nil {
		return err
	}
	uid, err := id(p.UserID)
	if err != nil {
		return err
	}
	channels := make([]uuid.UUID, 0, len(p.MutedChannelIDs))
	for _, c := range p.MutedChannelIDs {
		cid, err := uuid.Parse(c)
		if err != nil {
			return fmt.Errorf("notify: muted channel %q: %w", c, err)
		}
		channels = append(channels, cid)
	}
	start, err := clockValue(p.DNDStart)
	if err != nil {
		return err
	}
	end, err := clockValue(p.DNDEnd)
	if err != nil {
		return err
	}
	kinds := p.DisabledKinds
	if kinds == nil {
		kinds = []string{}
	}
	err = q.UpsertPreferences(ctx, gen.UpsertPreferencesParams{
		UserID: uid, WorkspaceID: wid, DisabledKinds: kinds, MutedChannelIds: channels,
		DndStart: start, DndEnd: end, UpdatedAt: p.UpdatedAt.UTC(),
	})
	return wrap(err, "save preferences")
}

func toPreferences(row gen.NotifyPreference) domain.Preferences {
	p := domain.Preferences{
		UserID: row.UserID.String(), WorkspaceID: row.WorkspaceID.String(),
		DisabledKinds:   row.DisabledKinds,
		MutedChannelIDs: make([]string, len(row.MutedChannelIds)),
		DNDStart:        clockString(row.DndStart), DNDEnd: clockString(row.DndEnd),
		UpdatedAt: row.UpdatedAt.UTC(),
	}
	if p.DisabledKinds == nil {
		p.DisabledKinds = []string{}
	}
	for i, c := range row.MutedChannelIds {
		p.MutedChannelIDs[i] = c.String()
	}
	return p
}

// clockValue turns "HH:MM" into a time column value. "" is NULL.
func clockValue(s string) (pgtype.Time, error) {
	if s == "" {
		return pgtype.Time{}, nil
	}
	t, err := time.Parse("15:04", s)
	if err != nil {
		return pgtype.Time{}, fmt.Errorf("notify: clock %q: %w", s, err)
	}
	minutes := int64(t.Hour()*60 + t.Minute())
	return pgtype.Time{Microseconds: minutes * int64(time.Minute/time.Microsecond), Valid: true}, nil
}

// clockString turns a time column value into "HH:MM". NULL is "".
func clockString(t pgtype.Time) string {
	if !t.Valid {
		return ""
	}
	minutes := t.Microseconds / int64(time.Minute/time.Microsecond)
	return fmt.Sprintf("%02d:%02d", minutes/60, minutes%60)
}
