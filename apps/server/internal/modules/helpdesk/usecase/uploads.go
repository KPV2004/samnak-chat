package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/domain"
)

// URLTTL is how long upload and download URLs work (NFR-07).
const URLTTL = 15 * time.Minute

var errNoStorage = errors.New("helpdesk: object storage is not configured")

// UploadInput asks for permission to upload one file.
type UploadInput struct {
	FileName string
	Size     int64
	MimeType string
}

// Upload is the new file and the request the client makes to put its bytes
// into object storage.
type Upload struct {
	AttachmentID string
	Request      app.PresignedRequest
}

// RequestUpload records a file and presigns the upload of its bytes (HD-03).
// The file belongs to the caller until a ticket takes it.
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
	if s.d.Storage == nil {
		return Upload{}, errNoStorage
	}
	a := domain.Attachment{
		ID: newID(), WorkspaceID: p.WorkspaceID, UploaderID: p.UserID,
		FileName: name, Size: in.Size, MimeType: mimeType, CreatedAt: s.clock(),
	}
	a.ObjectKey = domain.ObjectKey(a.WorkspaceID, a.ID)
	req, err := s.d.Storage.PresignPut(ctx, a.ObjectKey, a.MimeType, a.Size, URLTTL)
	if err != nil {
		return Upload{}, fmt.Errorf("presign upload: %w", err)
	}
	if err := s.d.Repo.InsertAttachment(ctx, a); err != nil {
		return Upload{}, err
	}
	return Upload{AttachmentID: a.ID, Request: req}, nil
}

// AttachmentURL returns a download URL for a file the caller may see: their
// own file before a ticket takes it, or a file of a ticket they may see. Any
// other file, in any workspace, is not found (NFR-06).
func (s *Service) AttachmentURL(ctx context.Context, p app.Principal, attachmentID string) (app.PresignedRequest, error) {
	a, err := s.d.Repo.Attachment(ctx, p.WorkspaceID, attachmentID)
	if err != nil {
		return app.PresignedRequest{}, err
	}
	if a.TicketID == "" {
		if a.UploaderID != p.UserID {
			return app.PresignedRequest{}, fmt.Errorf("attachment: %w", app.ErrNotFound)
		}
	} else {
		t, err := s.d.Repo.Ticket(ctx, p.WorkspaceID, a.TicketID)
		if err != nil {
			return app.PresignedRequest{}, err
		}
		act, err := s.actor(ctx, p, t)
		if err != nil {
			return app.PresignedRequest{}, err
		}
		if !t.Visible(act) {
			return app.PresignedRequest{}, fmt.Errorf("attachment: %w", app.ErrNotFound)
		}
	}
	if s.d.Storage == nil {
		return app.PresignedRequest{}, errNoStorage
	}
	req, err := s.d.Storage.PresignGet(ctx, a.ObjectKey, a.FileName, URLTTL)
	if err != nil {
		return app.PresignedRequest{}, fmt.Errorf("presign download: %w", err)
	}
	return req, nil
}

// parseAttachmentIDs checks the IDs of a create: UUIDs, no repeats, at most
// domain.MaxAttachments.
func parseAttachmentIDs(raw []string) ([]string, error) {
	if len(raw) > domain.MaxAttachments {
		return nil, invalid("a ticket holds at most %d uploaded files", domain.MaxAttachments)
	}
	out := make([]string, 0, len(raw))
	seen := map[string]bool{}
	for _, r := range raw {
		u, err := uuid.Parse(r)
		if err != nil {
			return nil, invalid("attachment_ids must be UUIDs")
		}
		id := u.String()
		if seen[id] {
			return nil, invalid("attachment_ids lists a file twice")
		}
		seen[id] = true
		out = append(out, id)
	}
	return out, nil
}

// checkUploads makes sure every file is the caller's, is not used by a ticket
// yet, and has its bytes in storage with the size it was declared with. A file
// that is not the caller's is not found, so other people's IDs do not leak.
func (s *Service) checkUploads(ctx context.Context, p app.Principal, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	files, err := s.d.Repo.AttachmentsByIDs(ctx, p.WorkspaceID, ids)
	if err != nil {
		return err
	}
	if len(files) != len(ids) {
		return fmt.Errorf("attachment: %w", app.ErrNotFound)
	}
	for _, f := range files {
		if f.UploaderID != p.UserID {
			return fmt.Errorf("attachment: %w", app.ErrNotFound)
		}
	}
	if s.d.Storage == nil {
		return errNoStorage
	}
	for _, f := range files {
		if f.TicketID != "" {
			return invalid("attachment %s is already used by a ticket", f.ID)
		}
		size, err := s.d.Storage.Stat(ctx, f.ObjectKey)
		if errors.Is(err, app.ErrNotFound) {
			return invalid("attachment %s has not been uploaded", f.ID)
		}
		if err != nil {
			return fmt.Errorf("check upload: %w", err)
		}
		if size != f.Size {
			return invalid("attachment %s has %d bytes, expected %d", f.ID, size, f.Size)
		}
	}
	return nil
}
