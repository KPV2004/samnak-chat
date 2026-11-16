package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/chat/adapter/postgres/gen"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/chat/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/chat/usecase"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/database"
)

// hydrateOne fills the files and reactions of one message.
func (r *Repo) hydrateOne(ctx context.Context, q *gen.Queries, wid uuid.UUID, v usecase.MessageView) (usecase.MessageView, error) {
	list := []usecase.MessageView{v}
	if err := r.hydrate(ctx, q, wid, list); err != nil {
		return usecase.MessageView{}, err
	}
	return list[0], nil
}

// hydrate fills the files and reactions of views with two queries. Deleted
// messages keep neither.
func (r *Repo) hydrate(ctx context.Context, q *gen.Queries, wid uuid.UUID, views []usecase.MessageView) error {
	var mids []uuid.UUID
	index := map[uuid.UUID]int{}
	for i, v := range views {
		if v.Deleted() {
			continue
		}
		mid, err := id(v.ID)
		if err != nil {
			return err
		}
		mids = append(mids, mid)
		index[mid] = i
	}
	if len(mids) == 0 {
		return nil
	}
	files, err := q.ListMessageAttachments(ctx, gen.ListMessageAttachmentsParams{WorkspaceID: wid, MessageIds: mids})
	if err != nil {
		return wrap(err, "list attachments")
	}
	for _, f := range files {
		if f.MessageID == nil {
			continue
		}
		i := index[*f.MessageID]
		views[i].Attachments = append(views[i].Attachments, toAttachment(attachmentRow(f)))
	}
	reactions, err := q.ListReactions(ctx, gen.ListReactionsParams{WorkspaceID: wid, MessageIds: mids})
	if err != nil {
		return wrap(err, "list reactions")
	}
	for _, g := range reactions {
		i := index[g.MessageID]
		views[i].Reactions = append(views[i].Reactions, usecase.ReactionGroup{Emoji: g.Emoji, UserIDs: strs(g.UserIds)})
	}
	return nil
}

// attachmentRow converts the identical row types sqlc makes per query.
type attachmentRow = gen.ChatAttachment

func toAttachment(row gen.ChatAttachment) domain.Attachment {
	a := domain.Attachment{
		ID: row.ID.String(), WorkspaceID: row.WorkspaceID.String(), ChannelID: row.ChannelID.String(), UploaderID: row.UploaderID.String(),
		ObjectKey: row.ObjectKey, FileName: row.FileName, Size: row.Size, MimeType: row.MimeType, CreatedAt: row.CreatedAt.UTC(),
	}
	if row.MessageID != nil {
		a.MessageID = row.MessageID.String()
	}
	return a
}

func (r *Repo) InsertAttachment(ctx context.Context, a domain.Attachment) error {
	q, err := r.q(ctx)
	if err != nil {
		return err
	}
	p := gen.InsertAttachmentParams{ObjectKey: a.ObjectKey, FileName: a.FileName, Size: a.Size, MimeType: a.MimeType, CreatedAt: a.CreatedAt.UTC()}
	for _, f := range []struct {
		dst *uuid.UUID
		src string
	}{{&p.ID, a.ID}, {&p.WorkspaceID, a.WorkspaceID}, {&p.ChannelID, a.ChannelID}, {&p.UploaderID, a.UploaderID}} {
		if *f.dst, err = id(f.src); err != nil {
			return err
		}
	}
	return wrap(q.InsertAttachment(ctx, p), "insert attachment")
}

func (r *Repo) Attachment(ctx context.Context, workspaceID, attachmentID string) (domain.Attachment, error) {
	q, err := r.q(ctx)
	if err != nil {
		return domain.Attachment{}, err
	}
	wid, err := id(workspaceID)
	if err != nil {
		return domain.Attachment{}, err
	}
	aid, err := id(attachmentID)
	if err != nil {
		return domain.Attachment{}, err
	}
	row, err := q.GetAttachment(ctx, gen.GetAttachmentParams{WorkspaceID: wid, ID: aid})
	if err != nil {
		return domain.Attachment{}, wrap(err, "get attachment")
	}
	return toAttachment(gen.ChatAttachment(row)), nil
}

func (r *Repo) AttachmentsByIDs(ctx context.Context, workspaceID string, attachmentIDs []string) ([]domain.Attachment, error) {
	q, err := r.q(ctx)
	if err != nil {
		return nil, err
	}
	wid, err := id(workspaceID)
	if err != nil {
		return nil, err
	}
	aids, err := ids(attachmentIDs)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListAttachmentsByIDs(ctx, gen.ListAttachmentsByIDsParams{WorkspaceID: wid, Ids: aids})
	if err != nil {
		return nil, wrap(err, "list attachments")
	}
	out := make([]domain.Attachment, len(rows))
	for i, row := range rows {
		out[i] = toAttachment(gen.ChatAttachment(row))
	}
	return out, nil
}

func (r *Repo) AttachToMessage(ctx context.Context, workspaceID, channelID, uploaderID, messageID string, attachmentIDs []string) error {
	q, err := r.q(ctx)
	if err != nil {
		return err
	}
	p := gen.AttachToMessageParams{}
	for _, f := range []struct {
		dst *uuid.UUID
		src string
	}{{&p.WorkspaceID, workspaceID}, {&p.ChannelID, channelID}, {&p.UploaderID, uploaderID}} {
		if *f.dst, err = id(f.src); err != nil {
			return err
		}
	}
	mid, err := id(messageID)
	if err != nil {
		return err
	}
	p.MessageID = &mid
	if p.Ids, err = ids(attachmentIDs); err != nil {
		return err
	}
	n, err := q.AttachToMessage(ctx, p)
	if err != nil {
		return wrap(err, "attach files")
	}
	if n != int64(len(attachmentIDs)) {
		// Someone used a file since it was checked; the caller rolls back.
		return fmt.Errorf("an attachment is no longer available: %w", app.ErrConflict)
	}
	return nil
}

func (r *Repo) AddReaction(ctx context.Context, workspaceID, messageID, userID, emoji string, at time.Time) (bool, error) {
	q, err := r.q(ctx)
	if err != nil {
		return false, err
	}
	p := gen.InsertReactionParams{Emoji: emoji, CreatedAt: at.UTC()}
	for _, f := range []struct {
		dst *uuid.UUID
		src string
	}{{&p.WorkspaceID, workspaceID}, {&p.MessageID, messageID}, {&p.UserID, userID}} {
		if *f.dst, err = id(f.src); err != nil {
			return false, err
		}
	}
	n, err := q.InsertReaction(ctx, p)
	if err != nil {
		return false, wrap(err, "add reaction")
	}
	return n > 0, nil
}

func (r *Repo) RemoveReaction(ctx context.Context, workspaceID, messageID, userID, emoji string) (bool, error) {
	q, err := r.q(ctx)
	if err != nil {
		return false, err
	}
	p := gen.DeleteReactionParams{Emoji: emoji}
	for _, f := range []struct {
		dst *uuid.UUID
		src string
	}{{&p.WorkspaceID, workspaceID}, {&p.MessageID, messageID}, {&p.UserID, userID}} {
		if *f.dst, err = id(f.src); err != nil {
			return false, err
		}
	}
	n, err := q.DeleteReaction(ctx, p)
	if err != nil {
		return false, wrap(err, "remove reaction")
	}
	return n > 0, nil
}

func (r *Repo) UpdateMessagePayload(ctx context.Context, workspaceID, messageID string, payload json.RawMessage) error {
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
	n, err := q.UpdateMessagePayload(ctx, gen.UpdateMessagePayloadParams{WorkspaceID: wid, ID: mid, Payload: payload})
	if err != nil {
		return wrap(err, "update payload")
	}
	if n == 0 {
		return fmt.Errorf("card: %w", app.ErrNotFound)
	}
	return nil
}

func (r *Repo) MemberWorkspace(ctx context.Context, channelID, userID string) (string, error) {
	q, err := r.q(ctx)
	if err != nil {
		return "", err
	}
	cid, err := id(channelID)
	if err != nil {
		return "", err
	}
	uid, err := id(userID)
	if err != nil {
		return "", err
	}
	wid, err := q.GetMemberWorkspace(ctx, gen.GetMemberWorkspaceParams{ChannelID: cid, UserID: uid})
	if err != nil {
		return "", wrap(err, "get member")
	}
	return wid.String(), nil
}

// threadParticipantSQL lists who follows a thread: the author of the root
// message and everyone who replied under it. It is hand-written rather than
// sqlc because it is additive and chat's only cross-row list of users; the
// workspace and channel keep it inside one workspace and one channel.
//
// Deleted replies still count: their authors took part in the thread. The
// caller drops users who are no longer members of the channel.
const threadParticipantSQL = `
SELECT DISTINCT m.sender_id::text
FROM chat_messages m
WHERE m.workspace_id = $1 AND m.channel_id = $2
  AND (m.id = $3 OR m.thread_root_id = $3)
ORDER BY m.sender_id::text`

func (r *Repo) ThreadParticipantIDs(ctx context.Context, workspaceID, channelID, rootMessageID string) ([]string, error) {
	if r.pool == nil {
		return nil, errors.New("chat: database is not configured")
	}
	wid, err := id(workspaceID)
	if err != nil {
		return nil, err
	}
	cid, err := id(channelID)
	if err != nil {
		return nil, err
	}
	root, err := id(rootMessageID)
	if err != nil {
		return nil, err
	}
	rows, err := database.Querier(ctx, r.pool).Query(ctx, threadParticipantSQL, wid, cid, root)
	if err != nil {
		return nil, wrap(err, "list thread participants")
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, wrap(err, "read thread participant")
		}
		out = append(out, userID)
	}
	if err := rows.Err(); err != nil {
		return nil, wrap(err, "list thread participants")
	}
	return out, nil
}
