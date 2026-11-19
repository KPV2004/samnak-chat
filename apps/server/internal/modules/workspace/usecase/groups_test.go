package usecase_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace/usecase"
)

type fakeTx struct{}

func (fakeTx) Do(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

// fakeRepo implements only what the group use cases call; the embedded nil
// interface panics on anything else, which would be a bug in the test.
type fakeRepo struct {
	usecase.Repository
	active   map[string]bool
	replaced []usecase.GroupMember
	calls    []string
}

func (r *fakeRepo) LockWorkspace(context.Context, string) error {
	r.calls = append(r.calls, "lock")
	return nil
}

func (r *fakeRepo) GetGroup(_ context.Context, _, id string) (usecase.GroupView, error) {
	if id != "known" {
		return usecase.GroupView{}, app.ErrNotFound
	}
	return usecase.GroupView{}, nil
}

func (r *fakeRepo) ActiveMemberIDs(_ context.Context, _ string, ids []string) ([]string, error) {
	var out []string
	for _, id := range ids {
		if r.active[id] {
			out = append(out, id)
		}
	}
	return out, nil
}

func (r *fakeRepo) InsertAudit(context.Context, domain.AuditEntry) error { return nil }

func (r *fakeRepo) ReplaceGroupMembers(_ context.Context, _, _ string, m []usecase.GroupMember, _ time.Time) error {
	r.calls = append(r.calls, "replace")
	r.replaced = m
	return nil
}

type noDirectory struct{}

func (noDirectory) Users(context.Context, []string) (map[string]usecase.User, error) {
	return nil, errors.New("down")
}

func newService(r *fakeRepo) *usecase.Service {
	return usecase.New(fakeTx{}, r, noDirectory{}, nil, slog.New(slog.DiscardHandler), time.Now)
}

func TestReplaceGroupMembersRules(t *testing.T) {
	a, b, gone := uuid.NewString(), uuid.NewString(), uuid.NewString()
	admin := app.Principal{UserID: uuid.NewString(), WorkspaceID: uuid.NewString(), Role: app.RoleAdmin}
	member := admin
	member.Role = app.RoleMember

	cases := []struct {
		name    string
		p       app.Principal
		group   string
		members []usecase.GroupMember
		want    error
		stored  bool
	}{
		{"admin replaces", admin, "known", []usecase.GroupMember{{UserID: a}, {UserID: b, IsManager: true}}, nil, true},
		{"empty set", admin, "known", nil, nil, true},
		{"plain member", member, "known", []usecase.GroupMember{{UserID: a}}, app.ErrForbidden, false},
		{"inactive user", admin, "known", []usecase.GroupMember{{UserID: a}, {UserID: gone}}, app.ErrInvalid, false},
		{"not a uuid", admin, "known", []usecase.GroupMember{{UserID: "x"}}, app.ErrInvalid, false},
		{"duplicate", admin, "known", []usecase.GroupMember{{UserID: a}, {UserID: a}}, app.ErrInvalid, false},
		{"unknown group", admin, "other", []usecase.GroupMember{{UserID: a}}, app.ErrNotFound, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo := &fakeRepo{active: map[string]bool{a: true, b: true}}
			err := newService(repo).ReplaceGroupMembers(t.Context(), c.p, c.group, c.members)
			if c.want == nil && err != nil || c.want != nil && !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
			if c.stored != (repo.calls != nil && repo.calls[len(repo.calls)-1] == "replace") {
				t.Fatalf("calls = %v, stored = %v", repo.calls, c.stored)
			}
			if c.stored && len(repo.replaced) != len(c.members) {
				t.Fatalf("replaced = %v", repo.replaced)
			}
		})
	}
}

func TestGroupManagementIsForOwnersAndAdmins(t *testing.T) {
	s := newService(&fakeRepo{})
	for _, role := range []app.Role{app.RoleManager, app.RoleMember, app.RoleGuest} {
		p := app.Principal{UserID: "u", WorkspaceID: "w", Role: role}
		if _, err := s.CreateGroup(t.Context(), p, "Sales"); !errors.Is(err, app.ErrForbidden) {
			t.Errorf("%s creates: %v", role, err)
		}
		if _, err := s.RenameGroup(t.Context(), p, "g", "Sales"); !errors.Is(err, app.ErrForbidden) {
			t.Errorf("%s renames: %v", role, err)
		}
		if err := s.DeleteGroup(t.Context(), p, "g"); !errors.Is(err, app.ErrForbidden) {
			t.Errorf("%s deletes: %v", role, err)
		}
	}
	if _, err := s.CreateGroup(t.Context(), app.Principal{Role: app.RoleOwner}, "  "); !errors.Is(err, app.ErrInvalid) {
		t.Errorf("blank name: %v", err)
	}
}
