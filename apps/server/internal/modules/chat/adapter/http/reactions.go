package httpadapter

import (
	"context"
	"net/url"
	"strings"
)

type addReactionInput struct {
	ID   string `path:"id"`
	Body struct {
		Emoji string `json:"emoji" minLength:"1" maxLength:"32" doc:"One emoji, such as 👍. Plain text is rejected"`
	}
}

func (h *Handler) addReaction(ctx context.Context, in *addReactionInput) (*struct{}, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	if err := h.uc.AddReaction(ctx, p, in.ID, in.Body.Emoji); err != nil {
		return nil, h.fail(ctx, err)
	}
	return nil, nil
}

type removeReactionInput struct {
	ID    string `path:"id"`
	Emoji string `path:"emoji" doc:"The emoji, percent-encoded"`
}

func (h *Handler) removeReaction(ctx context.Context, in *removeReactionInput) (*struct{}, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	emoji := in.Emoji
	// The router hands over the raw path segment when it was encoded in an
	// unusual way; an emoji never contains a literal '%'.
	if strings.Contains(emoji, "%") {
		if dec, err := url.PathUnescape(emoji); err == nil {
			emoji = dec
		}
	}
	if err := h.uc.RemoveReaction(ctx, p, in.ID, emoji); err != nil {
		return nil, h.fail(ctx, err)
	}
	return nil, nil
}
