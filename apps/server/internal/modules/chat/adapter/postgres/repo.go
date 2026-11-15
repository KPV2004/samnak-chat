// Package postgres implements the chat Repository with sqlc queries.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/chat/adapter/postgres/gen"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/chat/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/chat/usecase"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/database"
)

const uniqueViolation = "23505"

// Repo implements usecase.Repository. It runs on the transaction in the
// context when there is one, otherwise on the pool.
type Repo struct {
	pool *pgxpool.Pool
}

var _ usecase.Repository = (*Repo)(nil)

// New returns a Repo. pool may be nil when the process builds the OpenAPI
// document without a database; every call then fails with an error.
func New(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

func (r *Repo) q(ctx context.Context) (*gen.Queries, error) {
	if r.pool == nil {
		return nil, errors.New("chat: database is not configured")
	}
	return gen.New(database.Querier(ctx, r.pool)), nil
}

// id parses an ID. Text that is not a UUID cannot match any row, so it is
// reported as not found instead of reaching the database.
func id(s string) (uuid.UUID, error) {
	u, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, fmt.Errorf("%q: %w", s, app.ErrNotFound)
	}
	return u, nil
}

func ids(list []string) ([]uuid.UUID, error) {
	out := make([]uuid.UUID, len(list))
	for i, s := range list {
		u, err := id(s)
		if err != nil {
			return nil, err
		}
		out[i] = u
	}
	return out, nil
}

func strs(list []uuid.UUID) []string {
	out := make([]string, len(list))
	for i, u := range list {
		out[i] = u.String()
	}
	return out
}

func wrap(err error, what string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%s: %w", what, app.ErrNotFound)
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == uniqueViolation {
		return fmt.Errorf("%s: %w", what, app.ErrConflict)
	}
	return fmt.Errorf("chat: %s: %w", what, err)
}

func (r *Repo) InsertChannel(ctx context.Context, c domain.Channel) error {
	q, err := r.q(ctx)
	if err != nil {
		return err
	}
	cid, err := id(c.ID)
	if err != nil {
		return err
	}
	wid, err := id(c.WorkspaceID)
	if err != nil {
		return err
	}
	by, err := id(c.CreatedBy)
	if err != nil {
		return err
	}
	var key *string
	if c.DMKey != "" {
		key = &c.DMKey
	}
	return wrap(q.InsertChannel(ctx, gen.InsertChannelParams{
		ID: cid, WorkspaceID: wid, Type: string(c.Type), Name: c.Name, Topic: c.Topic, DmKey: key, CreatedBy: by, CreatedAt: c.CreatedAt.UTC(),
	}), "insert channel")
}

func (r *Repo) DirectChannelID(ctx context.Context, workspaceID, dmKey string) (string, error) {
	q, err := r.q(ctx)
	if err != nil {
		return "", err
	}
	wid, err := id(workspaceID)
	if err != nil {
		return "", err
	}
	cid, err := q.GetDirectChannelID(ctx, gen.GetDirectChannelIDParams{WorkspaceID: wid, DmKey: &dmKey})
	if err != nil {
		return "", wrap(err, "get direct channel")
	}
	return cid.String(), nil
}

func (r *Repo) GetChannel(ctx context.Context, workspaceID, channelID string) (domain.Channel, error) {
	q, err := r.q(ctx)
	if err != nil {
		return domain.Channel{}, err
	}
	wid, err := id(workspaceID)
	if err != nil {
		return domain.Channel{}, err
	}
	cid, err := id(channelID)
	if err != nil {
		return domain.Channel{}, err
	}
	row, err := q.GetChannel(ctx, gen.GetChannelParams{WorkspaceID: wid, ID: cid})
	if err != nil {
		return domain.Channel{}, wrap(err, "get channel")
	}
	key := ""
	if row.DmKey != nil {
		key = *row.DmKey
	}
	return domain.Channel{
		ID: row.ID.String(), WorkspaceID: row.WorkspaceID.String(), Type: domain.ChannelType(row.Type), Name: row.Name, Topic: row.Topic,
		DMKey: key, CreatedBy: row.CreatedBy.String(), LastSeq: row.LastSeq, ArchivedAt: utc(row.ArchivedAt), CreatedAt: row.CreatedAt.UTC(),
	}, nil
}

func (r *Repo) UpdateChannel(ctx context.Context, c domain.Channel) error {
	q, err := r.q(ctx)
	if err != nil {
		return err
	}
	wid, err := id(c.WorkspaceID)
	if err != nil {
		return err
	}
	cid, err := id(c.ID)
	if err != nil {
		return err
	}
	n, err := q.UpdateChannel(ctx, gen.UpdateChannelParams{WorkspaceID: wid, ID: cid, Name: c.Name, Topic: c.Topic, ArchivedAt: c.ArchivedAt})
	if err != nil {
		return wrap(err, "update channel")
	}
	if n == 0 {
		return fmt.Errorf("channel: %w", app.ErrNotFound)
	}
	return nil
}

func (r *Repo) NextSeq(ctx context.Context, workspaceID, channelID string) (int64, error) {
	q, err := r.q(ctx)
	if err != nil {
		return 0, err
	}
	wid, err := id(workspaceID)
	if err != nil {
		return 0, err
	}
	cid, err := id(channelID)
	if err != nil {
		return 0, err
	}
	seq, err := q.NextSeq(ctx, gen.NextSeqParams{WorkspaceID: wid, ID: cid})
	return seq, wrap(err, "next seq")
}

func (r *Repo) ChannelViews(ctx context.Context, workspaceID, userID string, mode usecase.ViewMode, channelID string) ([]usecase.ChannelView, error) {
	q, err := r.q(ctx)
	if err != nil {
		return nil, err
	}
	wid, err := id(workspaceID)
	if err != nil {
		return nil, err
	}
	uid, err := id(userID)
	if err != nil {
		return nil, err
	}
	cid := uuid.Nil
	if mode == usecase.ViewOne {
		if cid, err = id(channelID); err != nil {
			return nil, err
		}
	}
	rows, err := q.ChannelViews(ctx, gen.ChannelViewsParams{UserID: uid, WorkspaceID: wid, Mode: string(mode), ChannelID: cid})
	if err != nil {
		return nil, wrap(err, "list channels")
	}
	out := make([]usecase.ChannelView, len(rows))
	var dmIDs []uuid.UUID
	byID := map[string]int{}
	for i, row := range rows {
		key := ""
		if row.DmKey != nil {
			key = *row.DmKey
		}
		out[i] = usecase.ChannelView{
			Channel: domain.Channel{
				ID: row.ID.String(), WorkspaceID: row.WorkspaceID.String(), Type: domain.ChannelType(row.Type), Name: row.Name, Topic: row.Topic,
				DMKey: key, CreatedBy: row.CreatedBy.String(), LastSeq: row.LastSeq, ArchivedAt: utc(row.ArchivedAt), CreatedAt: row.CreatedAt.UTC(),
			},
			IsMember: row.IsMember, LastReadSeq: row.LastReadSeq, MemberCount: row.MemberCount,
			UnreadCount: row.UnreadCount, MentionCount: row.MentionCount, DMUserIDs: []string{},
		}
		if row.Type == string(domain.TypeDirect) {
			dmIDs = append(dmIDs, row.ID)
			byID[row.ID.String()] = i
		}
	}
	if len(dmIDs) > 0 {
		members, err := q.ListMembersOfChannels(ctx, gen.ListMembersOfChannelsParams{WorkspaceID: wid, ChannelIds: dmIDs})
		if err != nil {
			return nil, wrap(err, "list direct message members")
		}
		for _, m := range members {
			i := byID[m.ChannelID.String()]
			out[i].DMUserIDs = append(out[i].DMUserIDs, m.UserID.String())
		}
	}
	return out, nil
}

func (r *Repo) AddMembers(ctx context.Context, workspaceID, channelID string, userIDs []string, at time.Time) ([]string, error) {
	q, err := r.q(ctx)
	if err != nil {
		return nil, err
	}
	wid, err := id(workspaceID)
	if err != nil {
		return nil, err
	}
	cid, err := id(channelID)
	if err != nil {
		return nil, err
	}
	var added []string
	for _, s := range userIDs {
		uid, err := id(s)
		if err != nil {
			return nil, err
		}
		n, err := q.InsertChannelMember(ctx, gen.InsertChannelMemberParams{ChannelID: cid, WorkspaceID: wid, UserID: uid, JoinedAt: at.UTC()})
		if err != nil {
			return nil, wrap(err, "add member")
		}
		if n > 0 {
			added = append(added, s)
		}
	}
	return added, nil
}

func (r *Repo) RemoveMember(ctx context.Context, workspaceID, channelID, userID string) (bool, error) {
	q, err := r.q(ctx)
	if err != nil {
		return false, err
	}
	wid, err := id(workspaceID)
	if err != nil {
		return false, err
	}
	cid, err := id(channelID)
	if err != nil {
		return false, err
	}
	uid, err := id(userID)
	if err != nil {
		return false, err
	}
	n, err := q.DeleteChannelMember(ctx, gen.DeleteChannelMemberParams{WorkspaceID: wid, ChannelID: cid, UserID: uid})
	return n > 0, wrap(err, "remove member")
}

func (r *Repo) RemoveUserEverywhere(ctx context.Context, workspaceID, userID string) ([]string, error) {
	q, err := r.q(ctx)
	if err != nil {
		return nil, err
	}
	wid, err := id(workspaceID)
	if err != nil {
		return nil, err
	}
	uid, err := id(userID)
	if err != nil {
		return nil, err
	}
	rows, err := q.DeleteUserChannelMemberships(ctx, gen.DeleteUserChannelMembershipsParams{WorkspaceID: wid, UserID: uid})
	if err != nil {
		return nil, wrap(err, "remove user from channels")
	}
	return strs(rows), nil
}

func (r *Repo) MemberIDs(ctx context.Context, workspaceID, channelID string) ([]string, error) {
	members, err := r.Members(ctx, workspaceID, channelID)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(members))
	for i, m := range members {
		out[i] = m.UserID
	}
	return out, nil
}

func (r *Repo) Members(ctx context.Context, workspaceID, channelID string) ([]usecase.Member, error) {
	q, err := r.q(ctx)
	if err != nil {
		return nil, err
	}
	wid, err := id(workspaceID)
	if err != nil {
		return nil, err
	}
	cid, err := id(channelID)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListChannelMembers(ctx, gen.ListChannelMembersParams{WorkspaceID: wid, ChannelID: cid})
	if err != nil {
		return nil, wrap(err, "list members")
	}
	out := make([]usecase.Member, len(rows))
	for i, row := range rows {
		out[i] = usecase.Member{UserID: row.UserID.String(), JoinedAt: row.JoinedAt.UTC()}
	}
	return out, nil
}

func (r *Repo) FilterMembers(ctx context.Context, workspaceID, channelID string, userIDs []string) ([]string, error) {
	if len(userIDs) == 0 {
		return []string{}, nil
	}
	q, err := r.q(ctx)
	if err != nil {
		return nil, err
	}
	wid, err := id(workspaceID)
	if err != nil {
		return nil, err
	}
	cid, err := id(channelID)
	if err != nil {
		return nil, err
	}
	uids, err := ids(userIDs)
	if err != nil {
		return nil, err
	}
	rows, err := q.FilterChannelMembers(ctx, gen.FilterChannelMembersParams{WorkspaceID: wid, ChannelID: cid, UserIds: uids})
	if err != nil {
		return nil, wrap(err, "filter members")
	}
	return strs(rows), nil
}

func (r *Repo) AdvanceRead(ctx context.Context, workspaceID, channelID, userID string, seq int64) (int64, bool, error) {
	q, err := r.q(ctx)
	if err != nil {
		return 0, false, err
	}
	wid, err := id(workspaceID)
	if err != nil {
		return 0, false, err
	}
	cid, err := id(channelID)
	if err != nil {
		return 0, false, err
	}
	uid, err := id(userID)
	if err != nil {
		return 0, false, err
	}
	last, err := q.AdvanceRead(ctx, gen.AdvanceReadParams{Seq: seq, WorkspaceID: wid, ChannelID: cid, UserID: uid})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, wrap(err, "advance read")
	}
	return last, true, nil
}

func (r *Repo) InsertMessage(ctx context.Context, m domain.Message) error {
	q, err := r.q(ctx)
	if err != nil {
		return err
	}
	p := gen.InsertMessageParams{
		Seq: m.Seq, Kind: m.Kind, Body: m.Body, Payload: m.Payload, FallbackText: m.FallbackText,
		MentionsChannel: m.MentionsChannel, CreatedAt: m.CreatedAt.UTC(),
	}
	for _, f := range []struct {
		dst *uuid.UUID
		src string
	}{{&p.ID, m.ID}, {&p.WorkspaceID, m.WorkspaceID}, {&p.ChannelID, m.ChannelID}, {&p.SenderID, m.SenderID}, {&p.ClientMsgID, m.ClientMsgID}} {
		if *f.dst, err = id(f.src); err != nil {
			return err
		}
	}
	if m.ThreadRootID != "" {
		root, err := id(m.ThreadRootID)
		if err != nil {
			return err
		}
		p.ThreadRootID = &root
	}
	if p.Mentions, err = ids(m.Mentions); err != nil {
		return err
	}
	return wrap(q.InsertMessage(ctx, p), "insert message")
}

func (r *Repo) MessageByClientID(ctx context.Context, workspaceID, senderID, clientMsgID string) (usecase.MessageView, error) {
	q, err := r.q(ctx)
	if err != nil {
		return usecase.MessageView{}, err
	}
	wid, err := id(workspaceID)
	if err != nil {
		return usecase.MessageView{}, err
	}
	sid, err := id(senderID)
	if err != nil {
		return usecase.MessageView{}, err
	}
	cid, err := id(clientMsgID)
	if err != nil {
		return usecase.MessageView{}, err
	}
	row, err := q.GetMessageByClientID(ctx, gen.GetMessageByClientIDParams{WorkspaceID: wid, SenderID: sid, ClientMsgID: cid})
	if err != nil {
		return usecase.MessageView{}, wrap(err, "get message")
	}
	return toMessage(gen.GetMessageRow(row)), nil
}

func (r *Repo) Message(ctx context.Context, workspaceID, messageID string) (usecase.MessageView, error) {
	q, err := r.q(ctx)
	if err != nil {
		return usecase.MessageView{}, err
	}
	wid, err := id(workspaceID)
	if err != nil {
		return usecase.MessageView{}, err
	}
	mid, err := id(messageID)
	if err != nil {
		return usecase.MessageView{}, err
	}
	row, err := q.GetMessage(ctx, gen.GetMessageParams{WorkspaceID: wid, ID: mid})
	if err != nil {
		return usecase.MessageView{}, wrap(err, "get message")
	}
	return toMessage(row), nil
}

// threadFilter turns an optional root ID into the nullable query argument.
func threadFilter(threadRootID string) (*uuid.UUID, error) {
	if threadRootID == "" {
		return nil, nil
	}
	u, err := id(threadRootID)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *Repo) MessagesAfter(ctx context.Context, workspaceID, channelID string, afterSeq int64, threadRootID string, limit int) ([]usecase.MessageView, error) {
	q, err := r.q(ctx)
	if err != nil {
		return nil, err
	}
	wid, err := id(workspaceID)
	if err != nil {
		return nil, err
	}
	cid, err := id(channelID)
	if err != nil {
		return nil, err
	}
	root, err := threadFilter(threadRootID)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListMessagesAfter(ctx, gen.ListMessagesAfterParams{WorkspaceID: wid, ChannelID: cid, AfterSeq: afterSeq, ThreadRootID: root, PageSize: int32(limit)})
	if err != nil {
		return nil, wrap(err, "list messages")
	}
	out := make([]usecase.MessageView, len(rows))
	for i, row := range rows {
		out[i] = toMessage(gen.GetMessageRow(row))
	}
	return out, nil
}

func (r *Repo) MessagesBefore(ctx context.Context, workspaceID, channelID string, beforeSeq int64, threadRootID string, limit int) ([]usecase.MessageView, error) {
	q, err := r.q(ctx)
	if err != nil {
		return nil, err
	}
	wid, err := id(workspaceID)
	if err != nil {
		return nil, err
	}
	cid, err := id(channelID)
	if err != nil {
		return nil, err
	}
	root, err := threadFilter(threadRootID)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListMessagesBefore(ctx, gen.ListMessagesBeforeParams{WorkspaceID: wid, ChannelID: cid, BeforeSeq: beforeSeq, ThreadRootID: root, PageSize: int32(limit)})
	if err != nil {
		return nil, wrap(err, "list messages")
	}
	out := make([]usecase.MessageView, len(rows))
	for i, row := range rows {
		out[i] = toMessage(gen.GetMessageRow(row))
	}
	return out, nil
}

func (r *Repo) UpdateMessageBody(ctx context.Context, workspaceID, messageID, body string, mentions []string, mentionsChannel bool, at time.Time) error {
	q, err := r.q(ctx)
	if err != nil {
		return err
	}
	wid, err := id(workspaceID)
	if err != nil {
		return err
	}
	mid, err := id(messageID)
	if err != nil {
		return err
	}
	ms, err := ids(mentions)
	if err != nil {
		return err
	}
	t := at.UTC()
	n, err := q.UpdateMessageBody(ctx, gen.UpdateMessageBodyParams{WorkspaceID: wid, ID: mid, Body: body, Mentions: ms, MentionsChannel: mentionsChannel, EditedAt: &t})
	if err != nil {
		return wrap(err, "update message")
	}
	if n == 0 {
		return fmt.Errorf("message: %w", app.ErrNotFound)
	}
	return nil
}

func (r *Repo) SoftDeleteMessage(ctx context.Context, workspaceID, messageID string, at time.Time) error {
	q, err := r.q(ctx)
	if err != nil {
		return err
	}
	wid, err := id(workspaceID)
	if err != nil {
		return err
	}
	mid, err := id(messageID)
	if err != nil {
		return err
	}
	t := at.UTC()
	n, err := q.SoftDeleteMessage(ctx, gen.SoftDeleteMessageParams{WorkspaceID: wid, ID: mid, DeletedAt: &t})
	if err != nil {
		return wrap(err, "delete message")
	}
	if n == 0 {
		return fmt.Errorf("message: %w", app.ErrNotFound)
	}
	return nil
}

func toMessage(row gen.GetMessageRow) usecase.MessageView {
	m := domain.Message{
		ID: row.ID.String(), WorkspaceID: row.WorkspaceID.String(), ChannelID: row.ChannelID.String(), Seq: row.Seq,
		SenderID: row.SenderID.String(), ClientMsgID: row.ClientMsgID.String(), Kind: row.Kind, Body: row.Body,
		Payload: row.Payload, FallbackText: row.FallbackText, Mentions: strs(row.Mentions), MentionsChannel: row.MentionsChannel,
		EditedAt: utc(row.EditedAt), DeletedAt: utc(row.DeletedAt), CreatedAt: row.CreatedAt.UTC(),
	}
	if row.ThreadRootID != nil {
		m.ThreadRootID = row.ThreadRootID.String()
	}
	return usecase.MessageView{Message: m, ReplyCount: row.ReplyCount}
}

func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}
