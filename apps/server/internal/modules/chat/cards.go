package chat

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/chat/usecase"
)

// kindPattern is "<module>.<name>": lower-case words, at most 64 characters.
var kindPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*\.[a-z0-9][a-z0-9_.-]*$`)

// cardRegistry is the app.CardRegistry chat provides. It exists from Provide
// on, so other modules can look it up in Init; it reaches the use cases once
// chat's own Init has built them.
type cardRegistry struct {
	m     *Module
	mu    sync.RWMutex
	kinds map[string]app.CardKind
}

var _ app.CardRegistry = (*cardRegistry)(nil)

func newCardRegistry(m *Module) *cardRegistry {
	return &cardRegistry{m: m, kinds: map[string]app.CardKind{}}
}

func (r *cardRegistry) logger() *slog.Logger {
	if r.m.log != nil {
		return r.m.log
	}
	return slog.Default()
}

// Register declares a card kind. A kind must be "<module>.<name>" for the
// module that registers it, and a name another module owns is refused. Bad
// registrations are logged and ignored, so Post later fails for that kind.
func (r *cardRegistry) Register(k app.CardKind) {
	if k.Module == "" || len(k.Kind) > 64 || !kindPattern.MatchString(k.Kind) || !strings.HasPrefix(k.Kind, k.Module+".") || len(k.Kind) == len(k.Module)+1 {
		r.logger().Error("chat: card kind ignored: it must be <module>.<name> of the registering module", "kind", k.Kind, "module", k.Module)
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if have, ok := r.kinds[k.Kind]; ok && have.Module != k.Module {
		r.logger().Error("chat: card kind ignored: another module owns it", "kind", k.Kind, "module", k.Module, "owner", have.Module)
		return
	}
	r.kinds[k.Kind] = k
}

func (r *cardRegistry) registered(kind string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.kinds[kind]
	return ok
}

// service returns the use cases, which exist after chat's Init.
func (r *cardRegistry) service() (*usecase.Service, error) {
	if r.m.uc == nil {
		return nil, fmt.Errorf("chat has not been initialised: %w", app.ErrInvalid)
	}
	return r.m.uc, nil
}

func (r *cardRegistry) Post(ctx context.Context, c app.Card) (string, error) {
	if !r.registered(c.Kind) {
		return "", fmt.Errorf("card kind %q is not registered: %w", c.Kind, app.ErrInvalid)
	}
	payload, err := usecase.EncodePayload(c.Payload)
	if err != nil {
		return "", err
	}
	uc, err := r.service()
	if err != nil {
		return "", err
	}
	v, err := uc.PostCard(ctx, c, payload)
	if err != nil {
		return "", err
	}
	return v.ID, nil
}

// Update replaces a card's payload. chat cannot tell which module calls, so
// it checks the card's kind is registered, not who registered it.
func (r *cardRegistry) Update(ctx context.Context, workspaceID, messageID string, payload any) error {
	b, err := usecase.EncodePayload(payload)
	if err != nil {
		return err
	}
	uc, err := r.service()
	if err != nil {
		return err
	}
	return uc.UpdateCard(ctx, workspaceID, messageID, b, r.registered)
}

func (r *cardRegistry) DirectChannel(ctx context.Context, workspaceID string, userIDs []string) (string, error) {
	uc, err := r.service()
	if err != nil {
		return "", err
	}
	return uc.DirectChannel(ctx, workspaceID, userIDs)
}
