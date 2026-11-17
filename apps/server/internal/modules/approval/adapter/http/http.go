// Package httpadapter maps the approval use cases to Huma operations.
package httpadapter

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/approval/adapter/wire"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/approval/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/approval/usecase"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/httpx"
)

const tag = "approval"

// Handler serves the approval endpoints.
type Handler struct {
	uc  *usecase.Service
	log *slog.Logger
}

func New(uc *usecase.Service, log *slog.Logger) *Handler {
	if log == nil {
		log = slog.Default()
	}
	return &Handler{uc: uc, log: log}
}

// Register adds the endpoints to api, a group prefixed with "/approval". Every
// operation requires a workspace member, and the use cases scope the answer
// to the caller and the X-Workspace-ID workspace. Who may decide or cancel is
// the request's own rule, checked in the use case.
func (h *Handler) Register(api huma.API, services *app.Services) {
	member := huma.Middlewares{httpx.RequireMember(api, services)}
	op := func(id, method, path, summary string, status int) huma.Operation {
		return huma.Operation{
			OperationID: id, Method: method, Path: path, Summary: summary, DefaultStatus: status,
			Tags: []string{tag}, Security: httpx.BearerAuth, Middlewares: member,
		}
	}
	huma.Register(api, op("approval-create-request", http.MethodPost, "/requests", "Submit a leave or general request", http.StatusCreated), h.create)
	huma.Register(api, op("approval-list-requests", http.MethodGet, "/requests", "List my requests or the requests waiting for me, newest first", 0), h.list)
	huma.Register(api, op("approval-get-request", http.MethodGet, "/requests/{id}", "Get a request with its history and files; only its requester and approver see it", 0), h.get)
	huma.Register(api, op("approval-approve-request", http.MethodPost, "/requests/{id}/approve", "Approve a pending request; only its approver", 0), h.approve)
	huma.Register(api, op("approval-reject-request", http.MethodPost, "/requests/{id}/reject", "Reject a pending request with a reason; only its approver", 0), h.reject)
	huma.Register(api, op("approval-cancel-request", http.MethodPost, "/requests/{id}/cancel", "Cancel a pending request; only its requester", 0), h.cancel)
	huma.Register(api, op("approval-request-upload", http.MethodPost, "/uploads", "Presign the upload of a file of at most 50 MB for a request", http.StatusCreated), h.requestUpload)
	huma.Register(api, op("approval-get-attachment", http.MethodGet, "/attachments/{id}", "Get a presigned download URL for a file of a request", 0), h.attachmentURL)
}

func principal(ctx context.Context) (app.Principal, error) {
	p, ok := app.PrincipalFrom(ctx)
	if !ok {
		return app.Principal{}, app.ErrUnauthorized
	}
	return p, nil
}

// codeError is an error response with a code of its own, in the same JSON
// shape as every other error: {"error": {"code": ..., "message": ...}}.
type codeError struct {
	status int
	Err    codeErrorBody `json:"error"`
}

type codeErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *codeError) GetStatus() int { return e.status }
func (e *codeError) Error() string  { return e.Err.Message }

// fail maps contract errors to their status. Anything else is logged and
// answered as a plain 500 so internals never reach the client. A request
// without an approver is a 400 with the code approver_required (APR-02).
func (h *Handler) fail(ctx context.Context, err error) error {
	var status int
	switch {
	case domain.IsApproverRequired(err):
		return &codeError{status: http.StatusBadRequest, Err: codeErrorBody{Code: "approver_required", Message: err.Error()}}
	case errors.Is(err, app.ErrInvalid):
		status = http.StatusBadRequest
	case errors.Is(err, app.ErrUnauthorized):
		status = http.StatusUnauthorized
	case errors.Is(err, app.ErrForbidden):
		status = http.StatusForbidden
	case errors.Is(err, app.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, app.ErrConflict):
		status = http.StatusConflict
	default:
		h.log.ErrorContext(ctx, "approval: request failed", "request_id", httpx.RequestID(ctx), "error", err)
		return huma.NewError(http.StatusInternalServerError, "internal error")
	}
	return huma.NewError(status, err.Error())
}

type approvalCreateInput struct {
	Body struct {
		Type          string         `json:"type" enum:"leave,general"`
		Payload       map[string]any `json:"payload" doc:"leave: {leave_type annual|sick|personal|other, start_date, end_date (YYYY-MM-DD, end not before start), half_day? morning|afternoon (one day only), reason (1-1000)}; general: {subject (1-200), details (up to 4000), amount? (decimal string like \"1200.50\"), currency? (default THB)}"`
		ApproverID    string         `json:"approver_id,omitempty" doc:"Defaults to the first manager of one of your groups; 400 approver_required when there is none"`
		AttachmentIDs []string       `json:"attachment_ids,omitempty" maxItems:"10" doc:"Files from POST /approval/uploads that you uploaded; each can be used once"`
	}
}

type approvalDetailOutput struct {
	Body wire.ApprovalRequestDetail
}

func (h *Handler) create(ctx context.Context, in *approvalCreateInput) (*approvalDetailOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	payload, err := json.Marshal(in.Body.Payload)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	d, err := h.uc.Submit(ctx, p, usecase.SubmitInput{
		Type: in.Body.Type, Payload: payload, ApproverID: in.Body.ApproverID, AttachmentIDs: in.Body.AttachmentIDs,
	})
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &approvalDetailOutput{Body: wire.ToDetail(d)}, nil
}

type approvalListInput struct {
	Role   string `query:"role" enum:"requester,approver" doc:"requester: my requests; approver: waiting for me. Both when omitted"`
	Status string `query:"status" doc:"Comma-separated: pending, approved, rejected, cancelled"`
	Cursor string `query:"cursor" doc:"next_cursor of the previous page"`
	Limit  int    `query:"limit" default:"20" minimum:"1" maximum:"100" doc:"Page size"`
}

type approvalListOutput struct {
	Body struct {
		Items      []wire.ApprovalRequest `json:"items" nullable:"false"`
		NextCursor string                 `json:"next_cursor" doc:"Empty on the last page"`
	}
}

func (h *Handler) list(ctx context.Context, in *approvalListInput) (*approvalListOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	page, err := h.uc.List(ctx, p, usecase.ListQuery{Role: in.Role, Status: in.Status, Cursor: in.Cursor, Limit: in.Limit})
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	out := &approvalListOutput{}
	out.Body.Items = wire.ToRequests(page.Items)
	out.Body.NextCursor = page.NextCursor
	return out, nil
}

type approvalIDInput struct {
	ID string `path:"id"`
}

func (h *Handler) get(ctx context.Context, in *approvalIDInput) (*approvalDetailOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	d, err := h.uc.Get(ctx, p, in.ID)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &approvalDetailOutput{Body: wire.ToDetail(d)}, nil
}

func (h *Handler) approve(ctx context.Context, in *approvalIDInput) (*approvalDetailOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	d, err := h.uc.Approve(ctx, p, in.ID)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &approvalDetailOutput{Body: wire.ToDetail(d)}, nil
}

type approvalRejectInput struct {
	ID   string `path:"id"`
	Body struct {
		Reason string `json:"reason" doc:"1 to 1000 characters"`
	}
}

func (h *Handler) reject(ctx context.Context, in *approvalRejectInput) (*approvalDetailOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	d, err := h.uc.Reject(ctx, p, in.ID, in.Body.Reason)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &approvalDetailOutput{Body: wire.ToDetail(d)}, nil
}

func (h *Handler) cancel(ctx context.Context, in *approvalIDInput) (*approvalDetailOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	d, err := h.uc.Cancel(ctx, p, in.ID)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &approvalDetailOutput{Body: wire.ToDetail(d)}, nil
}

// ApprovalPresignedUpload is the request the client makes to object storage.
type ApprovalPresignedUpload struct {
	URL       string            `json:"url" doc:"Presigned URL; send the file bytes to it"`
	Method    string            `json:"method"`
	Headers   map[string]string `json:"headers" nullable:"false" doc:"Send exactly these headers; the URL is signed for them"`
	ExpiresAt time.Time         `json:"expires_at"`
}

type approvalUploadInput struct {
	Body struct {
		FileName string `json:"file_name" minLength:"1" maxLength:"1024"`
		Size     int64  `json:"size" minimum:"1" maximum:"52428800" doc:"Bytes, at most 50 MB"`
		MimeType string `json:"mime_type" minLength:"3" maxLength:"255" example:"application/pdf"`
	}
}

type approvalUploadOutput struct {
	Body struct {
		AttachmentID string                  `json:"attachment_id" doc:"Pass it in attachment_ids when submitting the request"`
		Upload       ApprovalPresignedUpload `json:"upload" doc:"Upload with PUT before submitting; expires after 15 minutes"`
	}
}

func (h *Handler) requestUpload(ctx context.Context, in *approvalUploadInput) (*approvalUploadOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	up, err := h.uc.RequestUpload(ctx, p, usecase.UploadInput{FileName: in.Body.FileName, Size: in.Body.Size, MimeType: in.Body.MimeType})
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	out := &approvalUploadOutput{}
	out.Body.AttachmentID = up.AttachmentID
	out.Body.Upload = ApprovalPresignedUpload{URL: up.Request.URL, Method: up.Request.Method, Headers: up.Request.Headers, ExpiresAt: up.Request.ExpiresAt.UTC()}
	if out.Body.Upload.Headers == nil {
		out.Body.Upload.Headers = map[string]string{}
	}
	return out, nil
}

type approvalAttachmentURLOutput struct {
	Body struct {
		URL       string    `json:"url" doc:"Presigned download URL; opening it saves the file under its name"`
		ExpiresAt time.Time `json:"expires_at"`
	}
}

func (h *Handler) attachmentURL(ctx context.Context, in *approvalIDInput) (*approvalAttachmentURLOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	req, err := h.uc.AttachmentURL(ctx, p, in.ID)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	out := &approvalAttachmentURLOutput{}
	out.Body.URL = req.URL
	out.Body.ExpiresAt = req.ExpiresAt.UTC()
	return out, nil
}
