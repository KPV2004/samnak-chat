package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/approval/domain"
)

// SubmitInput is a new request. ApproverID is optional (APR-02).
type SubmitInput struct {
	Type          string
	Payload       json.RawMessage
	ApproverID    string
	AttachmentIDs []string
}

// Submit validates and stores a request, its one step and its "created"
// history line, and publishes approval.request_created in the same
// transaction (APR-01, APR-02, APR-07, APR-13). The card and the notification
// follow from that event, after the commit.
func (s *Service) Submit(ctx context.Context, p app.Principal, in SubmitInput) (Detail, error) {
	typ, err := domain.ParseType(in.Type)
	if err != nil {
		return Detail{}, err
	}
	payload, err := domain.ParsePayload(typ, in.Payload)
	if err != nil {
		return Detail{}, err
	}
	fileIDs, err := parseAttachmentIDs(in.AttachmentIDs)
	if err != nil {
		return Detail{}, err
	}
	approver, err := s.resolveApprover(ctx, p, in.ApproverID)
	if err != nil {
		return Detail{}, err
	}
	if err := s.checkFiles(ctx, p, fileIDs); err != nil {
		return Detail{}, err
	}

	now := s.clock()
	req := domain.Request{
		ID: newID(), WorkspaceID: p.WorkspaceID, Type: typ, RequesterID: p.UserID, ApproverID: approver,
		Status: domain.StatusPending, Payload: payload.JSON, CreatedAt: now, UpdatedAt: now,
	}
	step := domain.Step{RequestID: req.ID, StepNo: 1, ApproverID: approver, Status: domain.StatusPending}
	hist := domain.HistoryEntry{ID: newID(), RequestID: req.ID, ActorID: p.UserID, Action: domain.ActionCreated, At: now}

	err = s.Tx.Do(ctx, func(ctx context.Context) error {
		if err := s.Repo.InsertRequest(ctx, req, step, hist); err != nil {
			return err
		}
		if len(fileIDs) > 0 {
			n, err := s.Repo.LinkAttachments(ctx, p.WorkspaceID, req.ID, p.UserID, fileIDs)
			if err != nil {
				return err
			}
			if n != int64(len(fileIDs)) {
				// Another submit took a file between the check and now.
				return fmt.Errorf("an attachment is already used by another request: %w", app.ErrConflict)
			}
		}
		return s.publish(ctx, domain.RequestCreated{
			WorkspaceID: req.WorkspaceID, RequestID: req.ID, Type: string(req.Type),
			RequesterID: req.RequesterID, ApproverID: req.ApproverID, ActorID: p.UserID,
		})
	})
	if err != nil {
		return Detail{}, err
	}
	return s.detail(ctx, req)
}

// resolveApprover returns the approver the request goes to: the one named, who
// must be an active member other than the requester, or else the first
// active group manager of the requester (APR-02).
func (s *Service) resolveApprover(ctx context.Context, p app.Principal, named string) (string, error) {
	if named != "" {
		u, err := uuid.Parse(named)
		if err != nil {
			return "", fmt.Errorf("approver_id must be a user ID: %w", app.ErrInvalid)
		}
		id := u.String()
		if id == p.UserID {
			return "", fmt.Errorf("you cannot approve your own request: %w", app.ErrInvalid)
		}
		if !s.active(ctx, p.WorkspaceID, id) {
			return "", fmt.Errorf("approver_id is not an active member of this workspace: %w", app.ErrInvalid)
		}
		return id, nil
	}
	if s.Managers == nil {
		return "", domain.ErrApproverRequired
	}
	managers, err := s.Managers.ManagersOf(ctx, p.WorkspaceID, p.UserID)
	if err != nil {
		return "", fmt.Errorf("approval: look up managers: %w", err)
	}
	for _, id := range managers {
		if id != p.UserID && s.active(ctx, p.WorkspaceID, id) {
			return id, nil
		}
	}
	return "", domain.ErrApproverRequired
}

func (s *Service) active(ctx context.Context, workspaceID, userID string) bool {
	return s.Members != nil && s.Members.Active(ctx, workspaceID, userID)
}

// parseAttachmentIDs checks the IDs of a submit: UUIDs, no duplicates, at
// most domain.MaxAttachments.
func parseAttachmentIDs(raw []string) ([]string, error) {
	if len(raw) > domain.MaxAttachments {
		return nil, fmt.Errorf("a request holds at most %d files: %w", domain.MaxAttachments, app.ErrInvalid)
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

var errNoStorage = errors.New("approval: object storage is not configured")

// checkFiles makes sure every file is the caller's, is unused, and that its
// bytes are in object storage (Stat). A file that is not the caller's is
// reported as not found, so IDs of other people's files do not leak.
func (s *Service) checkFiles(ctx context.Context, p app.Principal, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	files, err := s.Repo.AttachmentsByIDs(ctx, p.WorkspaceID, ids)
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
	if s.Storage == nil {
		return errNoStorage
	}
	for _, f := range files {
		if f.RequestID != "" {
			return fmt.Errorf("attachment %s is already used by a request: %w", f.ID, app.ErrInvalid)
		}
		size, err := s.Storage.Stat(ctx, f.ObjectKey)
		if errors.Is(err, app.ErrNotFound) {
			return fmt.Errorf("attachment %s has not been uploaded: %w", f.ID, app.ErrInvalid)
		}
		if err != nil {
			return fmt.Errorf("check upload: %w", err)
		}
		if size != f.Size {
			return fmt.Errorf("attachment %s has %d bytes, expected %d: %w", f.ID, size, f.Size, app.ErrInvalid)
		}
	}
	return nil
}
