// Package httpadapter maps the chat use cases to Huma operations.
package httpadapter

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/chat/adapter/wire"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/chat/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/chat/usecase"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/httpx"
)

const tag = "chat"

// Handler serves the chat endpoints.
type Handler struct {
	uc  *usecase.Service
	log *slog.Logger
}

func New(uc *usecase.Service, log *slog.Logger) *Handler { return &Handler{uc: uc, log: log} }

// Register adds the endpoints to api, a group prefixed with "/chat". Every
// operation requires a workspace member.
func (h *Handler) Register(api huma.API, services *app.Services) {
	member := huma.Middlewares{httpx.RequireMember(api, services)}
	op := func(id, method, path, summary string, status int) huma.Operation {
		return huma.Operation{
			OperationID: id, Method: method, Path: path, Summary: summary, DefaultStatus: status,
			Tags: []string{tag}, Security: httpx.BearerAuth, Middlewares: member,
		}
	}
	huma.Register(api, op("chat-list-channels", http.MethodGet, "/channels", "List my channels, or the public channels I can join", 0), h.listChannels)
	huma.Register(api, op("chat-create-channel", http.MethodPost, "/channels", "Create a public or private channel", http.StatusCreated), h.createChannel)
	huma.Register(api, op("chat-open-dm", http.MethodPost, "/dms", "Open a direct message; the same people always give the same channel", 0), h.openDM)
	huma.Register(api, op("chat-get-channel", http.MethodGet, "/channels/{id}", "Get a channel", 0), h.getChannel)
	huma.Register(api, op("chat-update-channel", http.MethodPatch, "/channels/{id}", "Rename, set the topic of, or archive a channel", 0), h.updateChannel)
	huma.Register(api, op("chat-join-channel", http.MethodPost, "/channels/{id}/join", "Join a public channel", http.StatusNoContent), h.join)
	huma.Register(api, op("chat-leave-channel", http.MethodPost, "/channels/{id}/leave", "Leave a channel", http.StatusNoContent), h.leave)
	huma.Register(api, op("chat-list-members", http.MethodGet, "/channels/{id}/members", "List the members of a channel", 0), h.listMembers)
	huma.Register(api, op("chat-add-members", http.MethodPost, "/channels/{id}/members", "Add workspace members to a channel", http.StatusNoContent), h.addMembers)
	huma.Register(api, op("chat-list-messages", http.MethodGet, "/channels/{id}/messages", "Catch up with after_seq, or page back with before_seq", 0), h.listMessages)
	huma.Register(api, op("chat-send-message", http.MethodPost, "/channels/{id}/messages", "Send a message; repeating client_msg_id returns the original with 200", 0), h.sendMessage)
	huma.Register(api, op("chat-edit-message", http.MethodPatch, "/messages/{id}", "Edit my message", 0), h.editMessage)
	huma.Register(api, op("chat-delete-message", http.MethodDelete, "/messages/{id}", "Delete my message (admins: any message)", http.StatusNoContent), h.deleteMessage)
	huma.Register(api, op("chat-add-reaction", http.MethodPost, "/messages/{id}/reactions", "React to a message with an emoji; repeating it changes nothing", http.StatusNoContent), h.addReaction)
	huma.Register(api, op("chat-remove-reaction", http.MethodDelete, "/messages/{id}/reactions/{emoji}", "Remove my reaction; removing one that is not there changes nothing", http.StatusNoContent), h.removeReaction)
	huma.Register(api, op("chat-mark-read", http.MethodPost, "/channels/{id}/read", "Move my read position forward", http.StatusNoContent), h.markRead)
}

func principal(ctx context.Context) (app.Principal, error) {
	p, ok := app.PrincipalFrom(ctx)
	if !ok {
		return app.Principal{}, app.ErrUnauthorized
	}
	return p, nil
}

// fail maps contract errors to their status. Anything else is logged and
// answered as a plain 500 so internals never reach the client.
func (h *Handler) fail(ctx context.Context, err error) error {
	var status int
	switch {
	case errors.Is(err, app.ErrInvalid):
		status = http.StatusBadRequest
	case errors.Is(err, app.ErrUnauthorized):
		status = http.StatusUnauthorized
	case errors.Is(err, app.ErrForbidden):
		status = http.StatusForbidden
	case errors.Is(err, app.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, app.ErrConflict):
		status = http.StatusConflict
	default:
		h.log.ErrorContext(ctx, "chat: request failed", "request_id", httpx.RequestID(ctx), "error", err)
		return huma.NewError(http.StatusInternalServerError, "internal error")
	}
	return huma.NewError(status, err.Error())
}

type channelOutput struct {
	Status int
	Body   wire.Channel
}

type idInput struct {
	ID string `path:"id"`
}

type listChannelsInput struct {
	Scope string `query:"scope" enum:"joined,public" default:"joined" doc:"joined: channels I am in; public: public channels I could join"`
}

type listChannelsOutput struct {
	Body struct {
		Items []wire.Channel `json:"items" nullable:"false"`
	}
}

func (h *Handler) listChannels(ctx context.Context, in *listChannelsInput) (*listChannelsOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	views, err := h.uc.ListChannels(ctx, p, usecase.Scope(in.Scope))
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	out := &listChannelsOutput{}
	out.Body.Items = make([]wire.Channel, len(views))
	for i, v := range views {
		out.Body.Items[i] = wire.ToChannel(v)
	}
	return out, nil
}

type createChannelInput struct {
	Body struct {
		Type      string   `json:"type" enum:"public,private"`
		Name      string   `json:"name" doc:"1 to 80 characters, unique in the workspace"`
		Topic     string   `json:"topic,omitempty" doc:"At most 250 characters"`
		MemberIDs []string `json:"member_ids,omitempty" maxItems:"100" doc:"Workspace members to add besides the caller"`
	}
}

func (h *Handler) createChannel(ctx context.Context, in *createChannelInput) (*channelOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	v, err := h.uc.CreateChannel(ctx, p, usecase.CreateChannelInput{
		Type: domain.ChannelType(in.Body.Type), Name: in.Body.Name, Topic: in.Body.Topic, MemberIDs: in.Body.MemberIDs,
	})
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &channelOutput{Status: http.StatusCreated, Body: wire.ToChannel(v)}, nil
}

type openDMInput struct {
	Body struct {
		UserIDs []string `json:"user_ids" minItems:"1" maxItems:"8" doc:"The other people; the caller is added"`
	}
}

func (h *Handler) openDM(ctx context.Context, in *openDMInput) (*channelOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	v, created, err := h.uc.OpenDirect(ctx, p, in.Body.UserIDs)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	return &channelOutput{Status: status, Body: wire.ToChannel(v)}, nil
}

func (h *Handler) getChannel(ctx context.Context, in *idInput) (*channelOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	v, err := h.uc.GetChannel(ctx, p, in.ID)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &channelOutput{Status: http.StatusOK, Body: wire.ToChannel(v)}, nil
}

type updateChannelInput struct {
	ID   string `path:"id"`
	Body struct {
		Name     *string `json:"name,omitempty"`
		Topic    *string `json:"topic,omitempty"`
		Archived *bool   `json:"archived,omitempty" doc:"true archives the channel, false restores it"`
	}
}

func (h *Handler) updateChannel(ctx context.Context, in *updateChannelInput) (*channelOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	v, err := h.uc.UpdateChannel(ctx, p, in.ID, usecase.ChannelPatch{Name: in.Body.Name, Topic: in.Body.Topic, Archived: in.Body.Archived})
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &channelOutput{Status: http.StatusOK, Body: wire.ToChannel(v)}, nil
}

func (h *Handler) join(ctx context.Context, in *idInput) (*struct{}, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	if err := h.uc.Join(ctx, p, in.ID); err != nil {
		return nil, h.fail(ctx, err)
	}
	return nil, nil
}

func (h *Handler) leave(ctx context.Context, in *idInput) (*struct{}, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	if err := h.uc.Leave(ctx, p, in.ID); err != nil {
		return nil, h.fail(ctx, err)
	}
	return nil, nil
}

type listMembersOutput struct {
	Body struct {
		Items []wire.ChannelMember `json:"items" nullable:"false"`
	}
}

func (h *Handler) listMembers(ctx context.Context, in *idInput) (*listMembersOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	members, err := h.uc.ListMembers(ctx, p, in.ID)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	out := &listMembersOutput{}
	out.Body.Items = make([]wire.ChannelMember, len(members))
	for i, m := range members {
		out.Body.Items[i] = wire.ToMember(m)
	}
	return out, nil
}

type addMembersInput struct {
	ID   string `path:"id"`
	Body struct {
		UserIDs []string `json:"user_ids" minItems:"1" maxItems:"100"`
	}
}

func (h *Handler) addMembers(ctx context.Context, in *addMembersInput) (*struct{}, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	if err := h.uc.AddMembers(ctx, p, in.ID, in.Body.UserIDs); err != nil {
		return nil, h.fail(ctx, err)
	}
	return nil, nil
}
