// Package wire holds the JSON shapes of the approval REST API. Type names
// start with "Approval" because Huma names schemas after their Go types and
// those names are shared by every module's operations.
package wire

import (
	"encoding/json"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/approval/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/approval/usecase"
)

// ApprovalRequest is one request as its requester and approver see it.
type ApprovalRequest struct {
	ID          string         `json:"id"`
	Type        string         `json:"type" enum:"leave,general"`
	Status      string         `json:"status" enum:"pending,approved,rejected,cancelled"`
	RequesterID string         `json:"requester_id"`
	ApproverID  string         `json:"approver_id"`
	Payload     map[string]any `json:"payload" nullable:"false" doc:"leave: {leave_type, start_date, end_date, half_day?, reason}; general: {subject, details, amount?, currency?}"`
	Reason      string         `json:"reason" doc:"Why the approver rejected it; empty otherwise"`
	DecidedAt   *time.Time     `json:"decided_at" doc:"When it was approved, rejected or cancelled"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	MessageID   *string        `json:"message_id" doc:"The card in the requester-approver DM; null until it is posted"`
}

// ApprovalHistoryEntry is one line of a request's history (APR-07).
type ApprovalHistoryEntry struct {
	ID      string    `json:"id"`
	ActorID string    `json:"actor_id"`
	Action  string    `json:"action" enum:"created,approved,rejected,cancelled"`
	Reason  string    `json:"reason"`
	At      time.Time `json:"at"`
}

// ApprovalAttachment is a file on a request (APR-09).
type ApprovalAttachment struct {
	ID        string    `json:"id" doc:"Pass it to GET /approval/attachments/{id} for a download URL"`
	FileName  string    `json:"file_name"`
	Size      int64     `json:"size"`
	MimeType  string    `json:"mime_type"`
	CreatedAt time.Time `json:"created_at"`
}

// ApprovalRequestDetail is a request with its history and files.
type ApprovalRequestDetail struct {
	ApprovalRequest
	History     []ApprovalHistoryEntry `json:"history" nullable:"false" doc:"Oldest first"`
	Attachments []ApprovalAttachment   `json:"attachments" nullable:"false"`
}

// ToRequest converts a stored request.
func ToRequest(r domain.Request) ApprovalRequest {
	payload := map[string]any{}
	if len(r.Payload) > 0 {
		// The column holds what ParsePayload produced; a value that does not
		// decode would be a bug in the writer, so an empty map is safe.
		_ = json.Unmarshal(r.Payload, &payload)
	}
	out := ApprovalRequest{
		ID: r.ID, Type: string(r.Type), Status: string(r.Status), RequesterID: r.RequesterID, ApproverID: r.ApproverID,
		Payload: payload, Reason: r.Reason, CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
	if r.DecidedAt != nil {
		t := r.DecidedAt.UTC()
		out.DecidedAt = &t
	}
	if r.MessageID != "" {
		id := r.MessageID
		out.MessageID = &id
	}
	return out
}

// ToRequests converts a page.
func ToRequests(rs []domain.Request) []ApprovalRequest {
	out := make([]ApprovalRequest, len(rs))
	for i, r := range rs {
		out[i] = ToRequest(r)
	}
	return out
}

// ToDetail converts a request with its history and files.
func ToDetail(d usecase.Detail) ApprovalRequestDetail {
	out := ApprovalRequestDetail{
		ApprovalRequest: ToRequest(d.Request),
		History:         make([]ApprovalHistoryEntry, len(d.History)),
		Attachments:     make([]ApprovalAttachment, len(d.Attachments)),
	}
	for i, h := range d.History {
		out.History[i] = ApprovalHistoryEntry{ID: h.ID, ActorID: h.ActorID, Action: string(h.Action), Reason: h.Reason, At: h.At.UTC()}
	}
	for i, a := range d.Attachments {
		out.Attachments[i] = ApprovalAttachment{ID: a.ID, FileName: a.FileName, Size: a.Size, MimeType: a.MimeType, CreatedAt: a.CreatedAt.UTC()}
	}
	return out
}
