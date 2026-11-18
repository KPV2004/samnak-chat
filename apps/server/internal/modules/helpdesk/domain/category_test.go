package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/domain"
)

const (
	catID  = "0193f0f0-0000-7000-8000-0000000000e1"
	queueB = "0193f0f0-0000-7000-8000-0000000000d2"
)

func TestNewCategoryNeedsAName(t *testing.T) {
	for _, name := range []string{"", "  ", strings.Repeat("a", domain.MaxNameRunes+1)} {
		_, err := domain.NewCategory(domain.CategoryInput{QueueID: qID, Name: name}, catID, ws, t0)
		if !errors.Is(err, app.ErrInvalid) {
			t.Errorf("name %q: err = %v, want ErrInvalid", name, err)
		}
	}
	c, err := domain.NewCategory(domain.CategoryInput{QueueID: qID, Name: " Hardware ", IsDefault: true}, catID, ws, t0)
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "Hardware" || !c.IsDefault || c.QueueID != qID || c.WorkspaceID != ws || c.ArchivedAt != nil {
		t.Errorf("category = %+v", c)
	}
}

func TestApplyMovesRenamesAndArchivesACategory(t *testing.T) {
	c, err := domain.NewCategory(domain.CategoryInput{QueueID: qID, Name: "Hardware", IsDefault: true}, catID, ws, t0)
	if err != nil {
		t.Fatal(err)
	}

	name, queue := "Laptops", queueB
	if err := c.Apply(domain.CategoryPatch{Name: &name, QueueID: &queue}, t0); err != nil {
		t.Fatal(err)
	}
	if c.Name != name || c.QueueID != queueB || !c.IsDefault {
		t.Errorf("after rename and move: %+v", c)
	}

	empty := ""
	if err := c.Apply(domain.CategoryPatch{QueueID: &empty}, t0); !errors.Is(err, app.ErrInvalid) {
		t.Errorf("empty queue_id: err = %v, want ErrInvalid", err)
	}

	archived := true
	if err := c.Apply(domain.CategoryPatch{Archived: &archived}, t0); err != nil {
		t.Fatal(err)
	}
	if c.ArchivedAt == nil || c.IsDefault {
		t.Errorf("archiving must set archived_at and drop the default: %+v", c)
	}
}

func TestApplySetsTheDefaultFlag(t *testing.T) {
	c, err := domain.NewCategory(domain.CategoryInput{QueueID: qID, Name: "Hardware"}, catID, ws, t0)
	if err != nil {
		t.Fatal(err)
	}
	yes := true
	if err := c.Apply(domain.CategoryPatch{IsDefault: &yes}, t0); err != nil {
		t.Fatal(err)
	}
	if !c.IsDefault {
		t.Error("default flag not set")
	}
}
