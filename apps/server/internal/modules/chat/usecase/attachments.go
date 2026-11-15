package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/chat/domain"
)

// URLTTL is how long upload and download URLs work (NFR-07).
const URLTTL = 15 * time.Minute

// UploadInput asks for permission to upload one file to a channel.
type UploadInput struct {
	ChannelID string
	FileName  string
	Size      int64
	MimeType  string
}

// Upload is the new attachment and the request the client must make to put
// the bytes into object storage.
type Upload struct {
	AttachmentID string
	Request      app.PresignedRequest
}

var errNoStorage = errors.New("chat: object storage is not configured")

// RequestUpload records an attachment and presigns the upload of its bytes
// (CHAT-14). The file belongs to the caller until a message uses it.
func (s *Service) RequestUpload(ctx context.Context, p app.Principal, in UploadInput) (Upload, error) {
	name, err := domain.NormalizeFileName(in.FileName)
	if err != nil {
		return Upload{}, err
	}
	mimeType, err := domain.NormalizeMimeType(in.MimeType)
	if err != nil {
		return Upload{}, err
	}
	if err := domain.ValidateUploadSize(in.Size); err != nil {
		return Upload{}, err
	}
	ch, err := s.memberChannel(ctx, p, in.ChannelID)
	if err != nil {
		return Upload{}, err
	}
	if ch.Archived() {
		return Upload{}, fmt.Errorf("the channel is archived: %w", app.ErrConflict)
	}
	if s.store == nil {
		return Upload{}, errNoStorage
	}
	a := domain.Attachment{
		ID: newID(), WorkspaceID: p.WorkspaceID, ChannelID: ch.ID, UploaderID: p.UserID,
		FileName: name, Size: in.Size, MimeType: mimeType, CreatedAt: s.clock(),
	}
	a.ObjectKey = domain.ObjectKey(a.WorkspaceID, a.ID)
	req, err := s.store.PresignPut(ctx, a.ObjectKey, a.MimeType, a.Size, URLTTL)
	if err != nil {
		return Upload{}, fmt.Errorf("presign upload: %w", err)
	}
	if err := s.repo.InsertAttachment(ctx, a); err != nil {
		return Upload{}, err
	}
	return Upload{AttachmentID: a.ID, Request: req}, nil
}

// AttachmentURL returns a download URL for a file the caller may see: one on
// a live message of a channel they can see, or their own file that is not
// sent yet. Everything else is not found, in any workspace (NFR-06).
func (s *Service) AttachmentURL(ctx context.Context, p app.Principal, attachmentID string) (app.PresignedRequest, error) {
	a, err := s.repo.Attachment(ctx, p.WorkspaceID, attachmentID)
	if err != nil {
		return app.PresignedRequest{}, err
	}
	if _, err := s.channel(ctx, p, a.ChannelID); err != nil {
		return app.PresignedRequest{}, err
	}
	if a.MessageID == "" {
		if a.UploaderID != p.UserID {
			return app.PresignedRequest{}, fmt.Errorf("attachment: %w", app.ErrNotFound)
		}
	} else if m, err := s.repo.Message(ctx, p.WorkspaceID, a.MessageID); err != nil {
		return app.PresignedRequest{}, err
	} else if m.Deleted() {
		return app.PresignedRequest{}, fmt.Errorf("attachment: %w", app.ErrNotFound)
	}
	if s.store == nil {
		return app.PresignedRequest{}, errNoStorage
	}
	req, err := s.store.PresignGet(ctx, a.ObjectKey, a.FileName, URLTTL)
	if err != nil {
		return app.PresignedRequest{}, fmt.Errorf("presign download: %w", err)
	}
	return req, nil
}

// parseAttachmentIDs checks the IDs of a send request: UUIDs, no duplicates,
// at most domain.MaxAttachments.
func parseAttachmentIDs(raw []string) ([]string, error) {
	if len(raw) > domain.MaxAttachments {
		return nil, fmt.Errorf("a message holds at most %d files: %w", domain.MaxAttachments, app.ErrInvalid)
	}
	out := make([]string, 0, len(raw))
	seen := map[string]bool{}
	for _, r := range raw {
		u, err := uuid.Parse(r)
		if err != nil {
			return nil, fmt.Errorf("attachment_ids must be UUIDs: %w", app.ErrInvalid)
		}
		id := u.String()
		if seen[id] {
			return nil, fmt.Errorf("attachment_ids lists a file twice: %w", app.ErrInvalid)
		}
		seen[id] = true
		out = append(out, id)
	}
	return out, nil
}

// checkFiles makes sure every file is the caller's, was uploaded to this
// channel, is unused, and that its bytes are in object storage (Stat). It
// returns the files in upload order. A file that is not the caller's is
// reported as not found, so IDs of other people's files do not leak.
func (s *Service) checkFiles(ctx context.Context, p app.Principal, channelID string, ids []string) ([]domain.Attachment, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	files, err := s.repo.AttachmentsByIDs(ctx, p.WorkspaceID, ids)
	if err != nil {
		return nil, err
	}
	if len(files) != len(ids) {
		return nil, fmt.Errorf("attachment: %w", app.ErrNotFound)
	}
	for _, f := range files {
		if f.UploaderID != p.UserID {
			return nil, fmt.Errorf("attachment: %w", app.ErrNotFound)
		}
	}
	if s.store == nil {
		return nil, errNoStorage
	}
	for _, f := range files {
		if f.ChannelID != channelID {
			return nil, fmt.Errorf("attachment %s was uploaded to another channel: %w", f.ID, app.ErrInvalid)
		}
		if f.MessageID != "" {
			return nil, fmt.Errorf("attachment %s is already used by a message: %w", f.ID, app.ErrInvalid)
		}
		size, err := s.store.Stat(ctx, f.ObjectKey)
		if errors.Is(err, app.ErrNotFound) {
			return nil, fmt.Errorf("attachment %s has not been uploaded: %w", f.ID, app.ErrInvalid)
		}
		if err != nil {
			return nil, fmt.Errorf("check upload: %w", err)
		}
		if size != f.Size {
			return nil, fmt.Errorf("attachment %s has %d bytes, expected %d: %w", f.ID, size, f.Size, app.ErrInvalid)
		}
	}
	return files, nil
}
