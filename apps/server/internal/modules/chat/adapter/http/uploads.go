package httpadapter

import (
	"context"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/chat/usecase"
)

// PresignedUpload is the request the client makes to object storage.
type PresignedUpload struct {
	URL       string            `json:"url" doc:"Presigned URL; send the file bytes to it"`
	Method    string            `json:"method"`
	Headers   map[string]string `json:"headers" nullable:"false" doc:"Send exactly these headers; the URL is signed for them"`
	ExpiresAt time.Time         `json:"expires_at"`
}

type uploadInput struct {
	Body struct {
		ChannelID string `json:"channel_id" doc:"A channel the caller is in"`
		FileName  string `json:"file_name" minLength:"1" maxLength:"1024"`
		Size      int64  `json:"size" minimum:"1" maximum:"52428800" doc:"Bytes, at most 50 MB"`
		MimeType  string `json:"mime_type" minLength:"3" maxLength:"255" example:"image/png"`
	}
}

type uploadOutput struct {
	Body struct {
		AttachmentID string          `json:"attachment_id" doc:"Pass it in attachment_ids when sending a message"`
		Upload       PresignedUpload `json:"upload" doc:"Upload with PUT before sending the message; expires after 15 minutes"`
	}
}

func (h *Handler) requestUpload(ctx context.Context, in *uploadInput) (*uploadOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	up, err := h.uc.RequestUpload(ctx, p, usecase.UploadInput{
		ChannelID: in.Body.ChannelID, FileName: in.Body.FileName, Size: in.Body.Size, MimeType: in.Body.MimeType,
	})
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	out := &uploadOutput{}
	out.Body.AttachmentID = up.AttachmentID
	out.Body.Upload = PresignedUpload{URL: up.Request.URL, Method: up.Request.Method, Headers: up.Request.Headers, ExpiresAt: up.Request.ExpiresAt.UTC()}
	if out.Body.Upload.Headers == nil {
		out.Body.Upload.Headers = map[string]string{}
	}
	return out, nil
}

type attachmentURLOutput struct {
	Body struct {
		URL       string    `json:"url" doc:"Presigned download URL; opening it saves the file under its name"`
		ExpiresAt time.Time `json:"expires_at"`
	}
}

func (h *Handler) attachmentURL(ctx context.Context, in *idInput) (*attachmentURLOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	req, err := h.uc.AttachmentURL(ctx, p, in.ID)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	out := &attachmentURLOutput{}
	out.Body.URL = req.URL
	out.Body.ExpiresAt = req.ExpiresAt.UTC()
	return out, nil
}
