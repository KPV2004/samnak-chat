package usecase

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace/domain"
)

// AuditPage is one page of the audit log. NextCursor is empty on the last page.
type AuditPage struct {
	Items      []domain.AuditEntry
	NextCursor string
}

// record writes the audit row of an admin action. It runs in the transaction
// of the change, so the row and the change commit together (WSP-10). nil data
// is stored as an empty object.
func (s *Service) record(ctx context.Context, p app.Principal, action domain.AuditAction, target domain.AuditTargetType, targetID string, data map[string]any) error {
	if data == nil {
		data = map[string]any{}
	}
	return s.repo.InsertAudit(ctx, domain.AuditEntry{
		ID: newID(), WorkspaceID: p.WorkspaceID, ActorID: p.UserID, Action: action,
		TargetType: target, TargetID: targetID, Data: data, CreatedAt: s.now().UTC(),
	})
}

// memberChanges is the data of a member.updated row. It holds the role and
// the active flag, each only when the request set it.
func memberChanges(before, after domain.Member, role *app.Role, status *domain.Status) map[string]any {
	data := map[string]any{}
	if role != nil {
		data["role"] = domain.Change[app.Role]{From: before.Role, To: after.Role}
	}
	if status != nil {
		data["active"] = domain.Change[bool]{From: before.Status == domain.StatusActive, To: after.Status == domain.StatusActive}
	}
	return data
}

func requireAuditReader(p app.Principal) error {
	if !domain.CanManage(p.Role) {
		return fmt.Errorf("only owners and admins read the audit log: %w", app.ErrForbidden)
	}
	return nil
}

// ListAudit returns a page of the principal's workspace audit log, newest
// first (WSP-10). Only owners and admins may read it.
func (s *Service) ListAudit(ctx context.Context, p app.Principal, cursor string, limit int) (AuditPage, error) {
	if err := requireAuditReader(p); err != nil {
		return AuditPage{}, err
	}
	after, err := decodeAuditCursor(cursor)
	if err != nil {
		return AuditPage{}, err
	}
	if limit < 1 {
		limit = defaultPageSize
	}
	limit = min(limit, maxPageSize)
	rows, err := s.repo.ListAudit(ctx, p.WorkspaceID, after, limit+1)
	if err != nil {
		return AuditPage{}, err
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		next = encodeAuditCursor(AuditCursor{CreatedAt: last.CreatedAt, ID: last.ID})
	}
	return AuditPage{Items: rows, NextCursor: next}, nil
}

// An audit cursor is base64url("<created_at RFC 3339 nano>|<row id>"). Clients
// treat it as opaque.
func encodeAuditCursor(c AuditCursor) string {
	raw := c.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + c.ID
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeAuditCursor(s string) (*AuditCursor, error) {
	if s == "" {
		return nil, nil
	}
	bad := fmt.Errorf("cursor is not valid: %w", app.ErrInvalid)
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, bad
	}
	at, id, ok := strings.Cut(string(raw), "|")
	if !ok {
		return nil, bad
	}
	t, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return nil, bad
	}
	if _, err := uuid.Parse(id); err != nil {
		return nil, bad
	}
	return &AuditCursor{CreatedAt: t, ID: id}, nil
}
