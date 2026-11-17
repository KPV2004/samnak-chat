package usecase

import (
	"context"
	"fmt"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/approval/domain"
)

// Notification kinds of the approval module (APR-08).
const (
	KindRequested = "approval.requested"
	KindApproved  = "approval.approved"
	KindRejected  = "approval.rejected"
	KindCancelled = "approval.cancelled"
)

// Notify is the notification rule for every approval event (APR-08): a new
// request tells the approver, a decision tells the requester, and a
// cancellation tells the approver. Keys are "approval:{request_id}:{action}",
// so a redelivered event adds nothing. What is said comes from the event, so
// the notice is right even when it runs after a later change.
func (s *Service) Notify(ctx context.Context, evt app.Event) ([]app.Notification, error) {
	var (
		workspaceID, requestID, typ, to, kind, key, title, tail string
		requester, approver                                     string
	)
	switch e := evt.(type) {
	case domain.RequestCreated:
		workspaceID, requestID, typ, requester, approver = e.WorkspaceID, e.RequestID, e.Type, e.RequesterID, e.ApproverID
		to, kind, key = approver, KindRequested, "created"
		title = s.nameOr(ctx, requester, "Someone") + " asked for your approval"
	case domain.RequestApproved:
		workspaceID, requestID, typ, requester, approver = e.WorkspaceID, e.RequestID, e.Type, e.RequesterID, e.ApproverID
		to, kind, key = requester, KindApproved, "approved"
		title = "Your request was approved"
	case domain.RequestRejected:
		workspaceID, requestID, typ, requester, approver = e.WorkspaceID, e.RequestID, e.Type, e.RequesterID, e.ApproverID
		to, kind, key = requester, KindRejected, "rejected"
		title = "Your request was rejected"
		tail = ". Reason: " + e.Reason
	case domain.RequestCancelled:
		workspaceID, requestID, typ, requester, approver = e.WorkspaceID, e.RequestID, e.Type, e.RequesterID, e.ApproverID
		to, kind, key = approver, KindCancelled, "cancelled"
		title = s.nameOr(ctx, requester, "Someone") + " cancelled a request"
	default:
		return nil, nil
	}

	summary := ""
	req, err := s.Repo.Request(ctx, workspaceID, requestID)
	switch {
	case err == nil:
		summary = req.Summary()
	case !isNotFound(err):
		return nil, fmt.Errorf("approval: notification summary: %w", err)
	}
	return []app.Notification{{
		WorkspaceID: workspaceID, UserID: to, Kind: kind, Title: title, Body: summary + tail,
		Link: "/approvals/" + requestID,
		Data: map[string]any{"request_id": requestID, "type": typ, "requester_id": requester, "approver_id": approver},
		Key:  "approval:" + requestID + ":" + key,
	}}, nil
}

func (s *Service) nameOr(ctx context.Context, id, fallback string) string {
	if n := s.names(ctx, id)[id]; n != "" {
		return n
	}
	return fallback
}
