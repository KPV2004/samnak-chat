package httpadapter

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace/usecase"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/httpx"
)

// registerGroups adds the group endpoints (WSP-09). Register calls it.
func (h *Handler) registerGroups(api huma.API, member huma.Middlewares) {
	huma.Register(api, huma.Operation{
		OperationID: "workspace-list-groups", Method: http.MethodGet, Path: "/groups",
		Summary: "List the groups of the workspace with member counts and manager IDs",
		Tags:    []string{tag}, Security: httpx.BearerAuth, Middlewares: member,
	}, h.listGroups)
	huma.Register(api, huma.Operation{
		OperationID: "workspace-create-group", Method: http.MethodPost, Path: "/groups", DefaultStatus: http.StatusCreated,
		Summary:     "Create a group (owner or admin)",
		Description: "Names are unique in the workspace, ignoring case: 409 when taken.",
		Tags:        []string{tag}, Security: httpx.BearerAuth, Middlewares: member,
	}, h.createGroup)
	huma.Register(api, huma.Operation{
		OperationID: "workspace-rename-group", Method: http.MethodPatch, Path: "/groups/{id}",
		Summary: "Rename a group (owner or admin)",
		Tags:    []string{tag}, Security: httpx.BearerAuth, Middlewares: member,
	}, h.renameGroup)
	huma.Register(api, huma.Operation{
		OperationID: "workspace-delete-group", Method: http.MethodDelete, Path: "/groups/{id}", DefaultStatus: http.StatusNoContent,
		Summary: "Delete a group and its memberships (owner or admin)",
		Tags:    []string{tag}, Security: httpx.BearerAuth, Middlewares: member,
	}, h.deleteGroup)
	huma.Register(api, huma.Operation{
		OperationID: "workspace-list-group-members", Method: http.MethodGet, Path: "/groups/{id}/members",
		Summary: "List the members of a group",
		Tags:    []string{tag}, Security: httpx.BearerAuth, Middlewares: member,
	}, h.listGroupMembers)
	huma.Register(api, huma.Operation{
		OperationID: "workspace-set-group-members", Method: http.MethodPut, Path: "/groups/{id}/members", DefaultStatus: http.StatusNoContent,
		Summary:     "Replace the members of a group (owner or admin)",
		Description: "The body is the whole new set, in one transaction. 400 when a user is not an active member of the workspace or is listed twice.",
		Tags:        []string{tag}, Security: httpx.BearerAuth, Middlewares: member,
	}, h.setGroupMembers)
}

// Group is a group with its counts.
type Group struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	MemberCount int      `json:"member_count"`
	ManagerIDs  []string `json:"manager_ids" nullable:"false" doc:"In the order they became managers"`
}

// GroupMember is someone in a group.
type GroupMember struct {
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name" doc:"Empty when the auth module is unavailable"`
	IsManager   bool   `json:"is_manager"`
}

func toGroup(v usecase.GroupView) Group {
	managers := v.ManagerIDs
	if managers == nil {
		managers = []string{}
	}
	return Group{ID: v.ID, Name: v.Name, MemberCount: v.MemberCount, ManagerIDs: managers}
}

type groupOutput struct{ Body Group }

type listGroupsOutput struct {
	Body struct {
		Items []Group `json:"items" nullable:"false"`
	}
}

func (h *Handler) listGroups(ctx context.Context, _ *struct{}) (*listGroupsOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	groups, err := h.uc.ListGroups(ctx, p)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	out := &listGroupsOutput{}
	out.Body.Items = make([]Group, len(groups))
	for i, g := range groups {
		out.Body.Items[i] = toGroup(g)
	}
	return out, nil
}

type groupNameBody struct {
	Name string `json:"name" maxLength:"100" doc:"1 to 100 characters, unique in the workspace ignoring case"`
}

type createGroupInput struct{ Body groupNameBody }

func (h *Handler) createGroup(ctx context.Context, in *createGroupInput) (*groupOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	g, err := h.uc.CreateGroup(ctx, p, in.Body.Name)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &groupOutput{Body: toGroup(g)}, nil
}

type renameGroupInput struct {
	ID   string `path:"id"`
	Body groupNameBody
}

func (h *Handler) renameGroup(ctx context.Context, in *renameGroupInput) (*groupOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	g, err := h.uc.RenameGroup(ctx, p, in.ID, in.Body.Name)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &groupOutput{Body: toGroup(g)}, nil
}

type groupIDInput struct {
	ID string `path:"id"`
}

func (h *Handler) deleteGroup(ctx context.Context, in *groupIDInput) (*struct{}, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	if err := h.uc.DeleteGroup(ctx, p, in.ID); err != nil {
		return nil, h.fail(ctx, err)
	}
	return nil, nil
}

type listGroupMembersOutput struct {
	Body struct {
		Items []GroupMember `json:"items" nullable:"false"`
	}
}

func (h *Handler) listGroupMembers(ctx context.Context, in *groupIDInput) (*listGroupMembersOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	members, err := h.uc.ListGroupMembers(ctx, p, in.ID)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	out := &listGroupMembersOutput{}
	out.Body.Items = make([]GroupMember, len(members))
	for i, m := range members {
		out.Body.Items[i] = GroupMember{UserID: m.UserID, DisplayName: m.DisplayName, IsManager: m.IsManager}
	}
	return out, nil
}

type setGroupMembersInput struct {
	ID   string `path:"id"`
	Body struct {
		Members []struct {
			UserID    string `json:"user_id" maxLength:"36"`
			IsManager bool   `json:"is_manager"`
		} `json:"members" nullable:"false" maxItems:"1000"`
	}
}

func (h *Handler) setGroupMembers(ctx context.Context, in *setGroupMembersInput) (*struct{}, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	members := make([]usecase.GroupMember, len(in.Body.Members))
	for i, m := range in.Body.Members {
		members[i] = usecase.GroupMember{UserID: m.UserID, IsManager: m.IsManager}
	}
	if err := h.uc.ReplaceGroupMembers(ctx, p, in.ID, members); err != nil {
		return nil, h.fail(ctx, err)
	}
	return nil, nil
}
