package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace/adapter/postgres/gen"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace/usecase"
)

func (r *Repo) InsertAudit(ctx context.Context, e domain.AuditEntry) error {
	q, err := r.q(ctx)
	if err != nil {
		return err
	}
	eid, err := id(e.ID)
	if err != nil {
		return err
	}
	wid, err := id(e.WorkspaceID)
	if err != nil {
		return err
	}
	aid, err := id(e.ActorID)
	if err != nil {
		return err
	}
	tid, err := id(e.TargetID)
	if err != nil {
		return err
	}
	data, err := json.Marshal(e.Data)
	if err != nil {
		return fmt.Errorf("workspace: audit data: %w", err)
	}
	return wrap(q.InsertAudit(ctx, gen.InsertAuditParams{
		ID: eid, WorkspaceID: wid, ActorID: aid, Action: string(e.Action), TargetType: string(e.TargetType),
		TargetID: tid, Data: data, CreatedAt: e.CreatedAt.UTC(),
	}), "insert audit")
}

func (r *Repo) ListAudit(ctx context.Context, workspaceID string, after *usecase.AuditCursor, limit int) ([]domain.AuditEntry, error) {
	q, err := r.q(ctx)
	if err != nil {
		return nil, err
	}
	wid, err := id(workspaceID)
	if err != nil {
		return nil, err
	}
	p := gen.ListAuditParams{WorkspaceID: wid, PageSize: int32(limit)}
	if after != nil {
		afterID, err := id(after.ID)
		if err != nil {
			return nil, fmt.Errorf("cursor: %w", app.ErrInvalid)
		}
		p.UseCursor, p.AfterCreatedAt, p.AfterID = true, after.CreatedAt.UTC(), afterID
	}
	rows, err := q.ListAudit(ctx, p)
	if err != nil {
		return nil, wrap(err, "list audit")
	}
	out := make([]domain.AuditEntry, len(rows))
	for i, row := range rows {
		var data map[string]any
		if err := json.Unmarshal(row.Data, &data); err != nil {
			return nil, fmt.Errorf("workspace: audit data: %w", err)
		}
		out[i] = domain.AuditEntry{
			ID: row.ID.String(), WorkspaceID: row.WorkspaceID.String(), ActorID: row.ActorID.String(),
			Action: domain.AuditAction(row.Action), TargetType: domain.AuditTargetType(row.TargetType),
			TargetID: row.TargetID.String(), Data: data, CreatedAt: row.CreatedAt.UTC(),
		}
	}
	return out, nil
}
