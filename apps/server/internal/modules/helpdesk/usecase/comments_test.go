package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/usecase"
)

func TestRequesterCommentsPublicallyAndCannotWriteInternal(t *testing.T) {
	e := newEnv(t)
	id, _ := e.workSetup(t)
	ctx := context.Background()

	c, err := e.svc.AddComment(ctx, e.p["bob"], id, usecase.CommentInput{Body: "Still jams"})
	if err != nil {
		t.Fatal(err)
	}
	if c.Internal || c.AuthorID != bob {
		t.Errorf("comment = %+v", c)
	}
	if _, err := e.svc.AddComment(ctx, e.p["bob"], id, usecase.CommentInput{Body: "secret", Internal: true}); !errors.Is(err, app.ErrForbidden) {
		t.Errorf("requester internal: err = %v, want ErrForbidden", err)
	}
	if _, err := e.svc.AddComment(ctx, e.p["bob"], id, usecase.CommentInput{Body: "  "}); !errors.Is(err, app.ErrInvalid) {
		t.Errorf("blank body: err = %v, want ErrInvalid", err)
	}
	if _, err := e.svc.AddComment(ctx, e.p["outside"], id, usecase.CommentInput{Body: "hi"}); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("stranger: err = %v, want ErrNotFound", err)
	}
}

func TestCommentsPublishEventsAndHistory(t *testing.T) {
	e := newEnv(t)
	id, _ := e.workSetup(t)
	ctx := context.Background()
	if _, err := e.svc.AddComment(ctx, e.p["bob"], id, usecase.CommentInput{Body: "Public"}); err != nil {
		t.Fatal(err)
	}
	internal, err := e.svc.AddComment(ctx, e.p["carol"], id, usecase.CommentInput{Body: "Check the driver", Internal: true})
	if err != nil {
		t.Fatal(err)
	}
	var added []domain.CommentAdded
	for _, evt := range e.bus.all() {
		if v, ok := evt.(domain.CommentAdded); ok {
			added = append(added, v)
		}
	}
	if len(added) != 2 || !added[1].Internal || added[1].CommentID != internal.ID || added[1].AuthorID != carol {
		t.Errorf("comment events = %+v", added)
	}
	last := e.repo.history[len(e.repo.history)-1]
	if last.Action != domain.ActionCommented || !last.Internal || last.ActorID != carol {
		t.Errorf("last history line = %+v, want an internal commented line by carol", last)
	}
}

func TestDetailHidesInternalCommentsAndHistoryFromTheRequester(t *testing.T) {
	e := newEnv(t)
	id, _ := e.workSetup(t)
	ctx := context.Background()
	if _, err := e.svc.AddComment(ctx, e.p["bob"], id, usecase.CommentInput{Body: "Public"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.AddComment(ctx, e.p["carol"], id, usecase.CommentInput{Body: "Internal", Internal: true}); err != nil {
		t.Fatal(err)
	}

	asRequester, err := e.svc.Get(ctx, e.p["bob"], id)
	if err != nil {
		t.Fatal(err)
	}
	if len(asRequester.Comments) != 1 || asRequester.Comments[0].Body != "Public" {
		t.Errorf("requester sees comments %+v", asRequester.Comments)
	}
	for _, h := range asRequester.History {
		if h.Internal {
			t.Errorf("requester sees an internal history line %+v", h)
		}
	}

	asAgent, err := e.svc.Get(ctx, e.p["carol"], id)
	if err != nil {
		t.Fatal(err)
	}
	if len(asAgent.Comments) != 2 {
		t.Errorf("agent sees %d comments, want 2", len(asAgent.Comments))
	}
	internalLines := 0
	for _, h := range asAgent.History {
		if h.Internal {
			internalLines++
		}
	}
	if internalLines != 1 {
		t.Errorf("agent sees %d internal history lines, want 1", internalLines)
	}
}
