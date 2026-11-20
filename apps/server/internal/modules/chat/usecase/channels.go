package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/chat/domain"
)

// Scope selects which channels ListChannels returns.
type Scope string

const (
	ScopeJoined Scope = "joined"
	ScopePublic Scope = "public"
)

// CreateChannelInput is a request to create a public or private channel.
type CreateChannelInput struct {
	Type      domain.ChannelType
	Name      string
	Topic     string
	MemberIDs []string
}

// ChannelPatch changes a channel; nil fields stay as they are.
type ChannelPatch struct {
	Name     *string
	Topic    *string
	Archived *bool
}

// ListChannels returns the channels the caller joined, or the public channels
// of the workspace they could join. Guests see only channels they were added
// to, so the public list is empty for them.
func (s *Service) ListChannels(ctx context.Context, p app.Principal, scope Scope) ([]ChannelView, error) {
	switch scope {
	case ScopeJoined:
		return s.repo.ChannelViews(ctx, p.WorkspaceID, p.UserID, ViewJoined, "")
	case ScopePublic:
		if p.Role == app.RoleGuest {
			return []ChannelView{}, nil
		}
		return s.repo.ChannelViews(ctx, p.WorkspaceID, p.UserID, ViewPublic, "")
	}
	return nil, fmt.Errorf("scope must be joined or public: %w", app.ErrInvalid)
}

// CreateChannel creates a channel with the caller and member_ids as members.
func (s *Service) CreateChannel(ctx context.Context, p app.Principal, in CreateChannelInput) (ChannelView, error) {
	if !s.perms.Can(p, domain.ActionChannelCreate) {
		return ChannelView{}, fmt.Errorf("you cannot create channels: %w", app.ErrForbidden)
	}
	if !domain.ValidCreateType(in.Type) {
		return ChannelView{}, fmt.Errorf("type must be public or private: %w", app.ErrInvalid)
	}
	name, err := domain.NormalizeChannelName(in.Name)
	if err != nil {
		return ChannelView{}, err
	}
	topic, err := domain.NormalizeTopic(in.Topic)
	if err != nil {
		return ChannelView{}, err
	}
	members, err := others(p.UserID, in.MemberIDs)
	if err != nil {
		return ChannelView{}, err
	}
	if err := s.checkWorkspaceMembers(ctx, p.WorkspaceID, members); err != nil {
		return ChannelView{}, err
	}
	now := s.clock()
	ch := domain.Channel{ID: newID(), WorkspaceID: p.WorkspaceID, Type: in.Type, Name: name, Topic: topic, CreatedBy: p.UserID, CreatedAt: now}
	err = s.tx.Do(ctx, func(ctx context.Context) error {
		if err := s.repo.InsertChannel(ctx, ch); err != nil {
			if errors.Is(err, app.ErrConflict) {
				return fmt.Errorf("a channel named %q already exists: %w", name, app.ErrConflict)
			}
			return err
		}
		_, err := s.repo.AddMembers(ctx, p.WorkspaceID, ch.ID, append([]string{p.UserID}, members...), now)
		return err
	})
	if err != nil {
		return ChannelView{}, err
	}
	return s.announce(ctx, p, ch.ID)
}

// OpenDirect returns the direct message with exactly the caller and userIDs,
// creating it when it does not exist. created tells which happened (CHAT-03).
func (s *Service) OpenDirect(ctx context.Context, p app.Principal, userIDs []string) (v ChannelView, created bool, err error) {
	others, err := others(p.UserID, userIDs)
	if err != nil {
		return ChannelView{}, false, err
	}
	key, members, err := domain.DirectKey(p.UserID, others)
	if err != nil {
		return ChannelView{}, false, err
	}
	if err := s.checkWorkspaceMembers(ctx, p.WorkspaceID, others); err != nil {
		return ChannelView{}, false, err
	}
	id, created, err := s.directChannel(ctx, p.WorkspaceID, p.UserID, key, members)
	if err != nil {
		return ChannelView{}, false, err
	}
	if created {
		v, err = s.announce(ctx, p, id)
		return v, true, err
	}
	v, err = s.channel(ctx, p, id)
	return v, false, err
}

// DirectChannel returns the direct message with exactly userIDs in a
// workspace, creating it when it does not exist. It is for modules that send
// a card to people and have no Principal, so every user must be an active
// member of the workspace.
func (s *Service) DirectChannel(ctx context.Context, workspaceID string, userIDs []string) (string, error) {
	if _, err := uuid.Parse(workspaceID); err != nil {
		return "", fmt.Errorf("workspace ID is not a valid ID: %w", app.ErrInvalid)
	}
	all, err := domain.ParseIDs(userIDs)
	if err != nil {
		return "", err
	}
	if len(all) < 2 {
		return "", fmt.Errorf("a direct message needs at least two different users: %w", app.ErrInvalid)
	}
	key, members, err := domain.DirectKey(all[0], all[1:])
	if err != nil {
		return "", err
	}
	if err := s.checkWorkspaceMembers(ctx, workspaceID, members); err != nil {
		return "", err
	}
	id, created, err := s.directChannel(ctx, workspaceID, all[0], key, members)
	if err != nil || !created {
		return id, err
	}
	views, err := s.repo.ChannelViews(ctx, workspaceID, all[0], ViewOne, id)
	if err != nil || len(views) == 0 {
		s.log.WarnContext(ctx, "chat: load new direct message", "channel_id", id, "error", err)
		return id, nil
	}
	s.announceTo(ctx, views[0])
	return id, nil
}

// directChannel finds the direct message with the given member key or creates
// it, and reports whether it did. members must already be checked.
func (s *Service) directChannel(ctx context.Context, workspaceID, creator, key string, members []string) (id string, created bool, err error) {
	id, err = s.repo.DirectChannelID(ctx, workspaceID, key)
	if err == nil {
		return id, false, nil
	}
	if !errors.Is(err, app.ErrNotFound) {
		return "", false, err
	}
	now := s.clock()
	ch := domain.Channel{ID: newID(), WorkspaceID: workspaceID, Type: domain.TypeDirect, DMKey: key, CreatedBy: creator, CreatedAt: now}
	err = s.tx.Do(ctx, func(ctx context.Context) error {
		if err := s.repo.InsertChannel(ctx, ch); err != nil {
			return err
		}
		_, err := s.repo.AddMembers(ctx, workspaceID, ch.ID, members, now)
		return err
	})
	if errors.Is(err, app.ErrConflict) {
		// Someone created the same direct message at the same time.
		id, err := s.repo.DirectChannelID(ctx, workspaceID, key)
		return id, false, err
	}
	if err != nil {
		return "", false, err
	}
	return ch.ID, true, nil
}

// announce loads the channel for p and tells its members it changed.
func (s *Service) announce(ctx context.Context, p app.Principal, channelID string) (ChannelView, error) {
	v, err := s.channel(ctx, p, channelID)
	if err != nil {
		return ChannelView{}, err
	}
	s.announceTo(ctx, v)
	return v, nil
}

func (s *Service) announceTo(ctx context.Context, v ChannelView, extra ...string) {
	ids, err := s.repo.MemberIDs(ctx, v.WorkspaceID, v.ID)
	if err != nil {
		s.log.WarnContext(ctx, "chat: list members for channel.updated", "channel_id", v.ID, "error", err)
		return
	}
	s.ev.ChannelUpdated(ctx, v, append(ids, extra...))
}

// GetChannel returns one channel the caller may see.
func (s *Service) GetChannel(ctx context.Context, p app.Principal, channelID string) (ChannelView, error) {
	return s.channel(ctx, p, channelID)
}

// UpdateChannel renames, sets the topic of, archives or unarchives a channel.
// The creator, owners and admins may do so (CHAT-02).
func (s *Service) UpdateChannel(ctx context.Context, p app.Principal, channelID string, patch ChannelPatch) (ChannelView, error) {
	v, err := s.channel(ctx, p, channelID)
	if err != nil {
		return ChannelView{}, err
	}
	if v.Type == domain.TypeDirect {
		return ChannelView{}, fmt.Errorf("a direct message cannot be edited: %w", app.ErrInvalid)
	}
	if v.CreatedBy != p.UserID && !s.perms.Can(p, domain.ActionChannelManage) {
		return ChannelView{}, fmt.Errorf("only the creator or an admin can edit a channel: %w", app.ErrForbidden)
	}
	ch := v.Channel
	if patch.Name != nil {
		if ch.Name, err = domain.NormalizeChannelName(*patch.Name); err != nil {
			return ChannelView{}, err
		}
	}
	if patch.Topic != nil {
		if ch.Topic, err = domain.NormalizeTopic(*patch.Topic); err != nil {
			return ChannelView{}, err
		}
	}
	if patch.Archived != nil {
		switch {
		case *patch.Archived && ch.ArchivedAt == nil:
			now := s.clock()
			ch.ArchivedAt = &now
		case !*patch.Archived:
			ch.ArchivedAt = nil
		}
	}
	if err := s.repo.UpdateChannel(ctx, ch); err != nil {
		if errors.Is(err, app.ErrConflict) {
			return ChannelView{}, fmt.Errorf("a channel named %q already exists: %w", ch.Name, app.ErrConflict)
		}
		return ChannelView{}, err
	}
	return s.announce(ctx, p, channelID)
}

// Join adds the caller to a public channel. Joining twice is fine.
func (s *Service) Join(ctx context.Context, p app.Principal, channelID string) error {
	v, err := s.channel(ctx, p, channelID)
	if err != nil {
		return err
	}
	if v.IsMember {
		return nil
	}
	if v.Archived() {
		return fmt.Errorf("the channel is archived: %w", app.ErrConflict)
	}
	// channel() only lets a non-member see public channels, so this is one.
	if _, err := s.repo.AddMembers(ctx, p.WorkspaceID, channelID, []string{p.UserID}, s.clock()); err != nil {
		return err
	}
	_, err = s.announce(ctx, p, channelID)
	return err
}

// Leave removes the caller from a channel. Leaving twice is fine; a direct
// message cannot be left.
func (s *Service) Leave(ctx context.Context, p app.Principal, channelID string) error {
	v, err := s.channel(ctx, p, channelID)
	if err != nil {
		return err
	}
	if v.Type == domain.TypeDirect {
		return fmt.Errorf("a direct message cannot be left: %w", app.ErrInvalid)
	}
	removed, err := s.repo.RemoveMember(ctx, p.WorkspaceID, channelID, p.UserID)
	if err != nil || !removed {
		return err
	}
	// The leaver's other devices must drop the channel too.
	if after, err := s.channel(ctx, p, channelID); err == nil {
		s.announceTo(ctx, after, p.UserID)
	} else {
		s.announceTo(ctx, v, p.UserID)
	}
	return nil
}

// ListMembers returns the members of a channel the caller may see.
func (s *Service) ListMembers(ctx context.Context, p app.Principal, channelID string) ([]Member, error) {
	if _, err := s.channel(ctx, p, channelID); err != nil {
		return nil, err
	}
	members, err := s.repo.Members(ctx, p.WorkspaceID, channelID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(members))
	for i, m := range members {
		ids[i] = m.UserID
	}
	profiles := s.dir.Profiles(ctx, ids)
	for i := range members {
		p := profiles[members[i].UserID]
		members[i].DisplayName = p.DisplayName
		members[i].AvatarURL = p.AvatarURL
	}
	return members, nil
}

// AddMembers adds workspace members to a channel the caller is in.
func (s *Service) AddMembers(ctx context.Context, p app.Principal, channelID string, userIDs []string) error {
	v, err := s.memberChannel(ctx, p, channelID)
	if err != nil {
		return err
	}
	if v.Type == domain.TypeDirect {
		return fmt.Errorf("members cannot be added to a direct message: %w", app.ErrInvalid)
	}
	if v.Archived() {
		return fmt.Errorf("the channel is archived: %w", app.ErrConflict)
	}
	ids, err := others(p.UserID, userIDs)
	if err != nil {
		return err
	}
	if err := s.checkWorkspaceMembers(ctx, p.WorkspaceID, ids); err != nil {
		return err
	}
	added, err := s.repo.AddMembers(ctx, p.WorkspaceID, channelID, ids, s.clock())
	if err != nil || len(added) == 0 {
		return err
	}
	_, err = s.announce(ctx, p, channelID)
	return err
}

// MemberRemoved takes a user out of every channel of the workspace after
// workspace.member_removed (WSP-04). It is safe to run twice.
func (s *Service) MemberRemoved(ctx context.Context, workspaceID, userID string) error {
	channels, err := s.repo.RemoveUserEverywhere(ctx, workspaceID, userID)
	if err != nil {
		return err
	}
	for _, cid := range channels {
		views, err := s.repo.ChannelViews(ctx, workspaceID, userID, ViewOne, cid)
		if err != nil || len(views) == 0 {
			continue
		}
		s.announceTo(ctx, views[0])
	}
	return nil
}
