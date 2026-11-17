package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/approval/domain"
)

// URLTTL is how long upload and download URLs work (NFR-07).
const URLTTL = 15 * time.Minute

// UploadInput asks for permission to upload one file.
type UploadInput struct {
	FileName string
	Size     int64
	MimeType string
}

// Upload is the new attachment and the request the client must make to put
// the bytes into object storage.
type Upload struct {
	AttachmentID string
	Request      app.PresignedRequest
}

// RequestUpload records an attachment and presigns the upload of its bytes
// (APR-09). The file belongs to the caller until a request uses it.
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
	if s.Storage == nil {
		return Upload{}, errNoStorage
	}
	a := domain.Attachment{
		ID: newID(), WorkspaceID: p.WorkspaceID, UploaderID: p.UserID,
		FileName: name, Size: in.Size, MimeType: mimeType, CreatedAt: s.clock(),
	}
	a.ObjectKey = domain.ObjectKey(a.WorkspaceID, a.ID)
	req, err := s.Storage.PresignPut(ctx, a.ObjectKey, a.MimeType, a.Size, URLTTL)
	if err != nil {
		return Upload{}, fmt.Errorf("presign upload: %w", err)
	}
	if err := s.Repo.InsertAttachment(ctx, a); err != nil {
		return Upload{}, err
	}
	return Upload{AttachmentID: a.ID, Request: req}, nil
}

// AttachmentURL returns a download URL for a file the caller may see: one on a
// request they sent or must decide, or their own file that is not submitted
// yet. Everything else is not found, in any workspace (NFR-06).
func (s *Service) AttachmentURL(ctx context.Context, p app.Principal, attachmentID string) (app.PresignedRequest, error) {
	a, err := s.Repo.Attachment(ctx, p.WorkspaceID, attachmentID)
	if err != nil {
		return app.PresignedRequest{}, err
	}
	if a.RequestID == "" {
		if a.UploaderID != p.UserID {
			return app.PresignedRequest{}, fmt.Errorf("attachment: %w", app.ErrNotFound)
		}
	} else {
		req, err := s.Repo.Request(ctx, p.WorkspaceID, a.RequestID)
		if err != nil {
			return app.PresignedRequest{}, err
		}
		if !req.CanSee(p.UserID) {
			return app.PresignedRequest{}, fmt.Errorf("attachment: %w", app.ErrNotFound)
		}
	}
	if s.Storage == nil {
		return app.PresignedRequest{}, errNoStorage
	}
	req, err := s.Storage.PresignGet(ctx, a.ObjectKey, a.FileName, URLTTL)
	if err != nil {
		return app.PresignedRequest{}, fmt.Errorf("presign download: %w", err)
	}
	return req, nil
}
