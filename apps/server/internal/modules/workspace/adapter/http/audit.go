package httpadapter

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/httpx"
)

// registerAudit adds the audit log endpoint (WSP-10). Register calls it.
func (h *Handler) registerAudit(api huma.API, member huma.Middlewares) {
	huma.Register(api, huma.Operation{
		OperationID: "workspace-list-audit", Method: http.MethodGet, Path: "/audit",
		Summary:     "List the audit log of the workspace, newest first (owner or admin)",
		Description: "Records invites, role and status changes, removals and group changes made by admins. Pages are at most 100 rows.",
		Tags:        []string{tag}, Security: httpx.BearerAuth, Middlewares: member,
	}, h.listAudit)
}

// AuditEntry is one row of the audit log.
type AuditEntry struct {
	ID         string         `json:"id"`
	ActorID    string         `json:"actor_id"`
	Action     string         `json:"action" enum:"invite.created,invite.revoked,member.updated,member.removed,group.created,group.updated,group.deleted,group.members_set"`
	TargetType string         `json:"target_type" enum:"member,invite,group"`
	TargetID   string         `json:"target_id"`
	Data       map[string]any `json:"data" doc:"What the action changed, for example {\"role\": {\"from\": \"member\", \"to\": \"manager\"}}; an empty object when nothing more is recorded"`
	CreatedAt  time.Time      `json:"created_at"`
}

func toAuditEntry(e domain.AuditEntry) AuditEntry {
	return AuditEntry{
		ID: e.ID, ActorID: e.ActorID, Action: string(e.Action), TargetType: string(e.TargetType),
		TargetID: e.TargetID, Data: e.Data, CreatedAt: e.CreatedAt.UTC(),
	}
}

type listAuditInput struct {
	Cursor string `query:"cursor" maxLength:"90" doc:"Opaque cursor from the previous page's next_cursor"`
	Limit  int    `query:"limit" default:"50" minimum:"1" maximum:"100"`
}

type listAuditOutput struct {
	Body struct {
		Items      []AuditEntry `json:"items" nullable:"false"`
		NextCursor string       `json:"next_cursor" doc:"Empty on the last page"`
	}
}

func (h *Handler) listAudit(ctx context.Context, in *listAuditInput) (*listAuditOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	page, err := h.uc.ListAudit(ctx, p, in.Cursor, in.Limit)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	out := &listAuditOutput{}
	out.Body.NextCursor = page.NextCursor
	out.Body.Items = make([]AuditEntry, len(page.Items))
	for i, e := range page.Items {
		out.Body.Items[i] = toAuditEntry(e)
	}
	return out, nil
}
