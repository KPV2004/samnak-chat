package usecase

import (
	"context"
	"fmt"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/approval/domain"
)

// Approve approves a pending request. Only its approver may (APR-04).
func (s *Service) Approve(ctx context.Context, p app.Principal, id string) (Detail, error) {
	return s.decide(ctx, p, id, domain.StatusApproved, "")
}

// Reject rejects a pending request with a reason. Only its approver may
// (APR-04).
func (s *Service) Reject(ctx context.Context, p app.Principal, id, reason string) (Detail, error) {
	return s.decide(ctx, p, id, domain.StatusRejected, reason)
}

// Cancel cancels a pending request. Only its requester may (APR-05).
func (s *Service) Cancel(ctx context.Context, p app.Principal, id string) (Detail, error) {
	return s.decide(ctx, p, id, domain.StatusCancelled, "")
}

// decide moves a pending request to its final status. The request, its step,
// the history line and the event change together or not at all. The write is
// conditional on "pending", so of two decisions at once one wins and the
// other gets 409 (APR-04).
//
// The order of refusals is: not found, not allowed (403), already decided
// (409), then a bad reason (400).
func (s *Service) decide(ctx context.Context, p app.Principal, id string, to domain.Status, reason string) (Detail, error) {
	req, err := s.Repo.Request(ctx, p.WorkspaceID, id)
	if err != nil {
		return Detail{}, err
	}
	if to == domain.StatusCancelled {
		err = req.CheckCancel(p.UserID)
	} else {
		err = req.CheckDecide(p.UserID)
	}
	if err != nil {
		return Detail{}, err
	}
	if to == domain.StatusRejected {
		if reason, err = domain.NormalizeReason(reason); err != nil {
			return Detail{}, err
		}
	}

	var decided domain.Request
	err = s.Tx.Do(ctx, func(ctx context.Context) error {
		now := s.clock()
		r, changed, err := s.Repo.Decide(ctx, p.WorkspaceID, id, to, reason, now)
		if err != nil {
			return err
		}
		if !changed {
			return fmt.Errorf("the request is already %s: %w", r.Status, app.ErrConflict)
		}
		decided = r
		hist := domain.HistoryEntry{ID: newID(), RequestID: id, ActorID: p.UserID, Action: action(to), Reason: reason, At: now}
		if err := s.Repo.InsertHistory(ctx, hist); err != nil {
			return err
		}
		return s.publish(ctx, decisionEvent(to, r, p.UserID, reason))
	})
	if err != nil {
		return Detail{}, err
	}
	return s.detail(ctx, decided)
}

func action(to domain.Status) domain.Action {
	switch to {
	case domain.StatusApproved:
		return domain.ActionApproved
	case domain.StatusRejected:
		return domain.ActionRejected
	}
	return domain.ActionCancelled
}

func decisionEvent(to domain.Status, r domain.Request, actorID, reason string) app.Event {
	typ := string(r.Type)
	switch to {
	case domain.StatusApproved:
		return domain.RequestApproved{WorkspaceID: r.WorkspaceID, RequestID: r.ID, Type: typ, RequesterID: r.RequesterID, ApproverID: r.ApproverID, ActorID: actorID}
	case domain.StatusRejected:
		return domain.RequestRejected{WorkspaceID: r.WorkspaceID, RequestID: r.ID, Type: typ, RequesterID: r.RequesterID, ApproverID: r.ApproverID, ActorID: actorID, Reason: reason}
	}
	return domain.RequestCancelled{WorkspaceID: r.WorkspaceID, RequestID: r.ID, Type: typ, RequesterID: r.RequesterID, ApproverID: r.ApproverID, ActorID: actorID}
}
