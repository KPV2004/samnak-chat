package usecase

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/chat/domain"
)

// commandName is what a slash command may be called: lower-case letters,
// digits and "-", 1 to 32 characters (CHAT-16).
var commandName = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)

// actionID is "<module>.<name>" in lower case, like a card kind.
var actionID = regexp.MustCompile(`^[a-z][a-z0-9_]*\.[a-z0-9][a-z0-9_.-]*$`)

// Extensions holds the slash commands and message actions other modules
// register (CHAT-16). It implements app.ChatExtensions and exists from
// chat's Provide on, before the use cases are built.
type Extensions struct {
	mu       sync.RWMutex
	commands map[string]app.Command
	actions  map[string]app.MessageAction
}

var _ app.ChatExtensions = (*Extensions)(nil)

func NewExtensions() *Extensions {
	return &Extensions{commands: map[string]app.Command{}, actions: map[string]app.MessageAction{}}
}

// RegisterCommand adds a command. It panics on an invalid or taken name, a
// missing module or a missing handler: those are programming errors that must
// stop the server at startup, and the contract has no error to return.
func (x *Extensions) RegisterCommand(c app.Command) {
	switch {
	case !commandName.MatchString(c.Name):
		panic(fmt.Sprintf("chat: command name %q must be 1 to 32 lower-case letters, digits or '-'", c.Name))
	case c.Module == "":
		panic(fmt.Sprintf("chat: command %q has no module", c.Name))
	case c.Handler == nil:
		panic(fmt.Sprintf("chat: command %q has no handler", c.Name))
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	if have, ok := x.commands[c.Name]; ok {
		panic(fmt.Sprintf("chat: command %q is already registered by module %q", c.Name, have.Module))
	}
	x.commands[c.Name] = c
}

// RegisterMessageAction adds a message action. It panics on an ID that is not
// "<module>.<name>" of the registering module, or one that is taken.
func (x *Extensions) RegisterMessageAction(a app.MessageAction) {
	if a.Module == "" || len(a.ID) > 64 || !actionID.MatchString(a.ID) || !strings.HasPrefix(a.ID, a.Module+".") || len(a.ID) == len(a.Module)+1 {
		panic(fmt.Sprintf("chat: message action %q must be <module>.<name> of module %q", a.ID, a.Module))
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	if _, ok := x.actions[a.ID]; ok {
		panic(fmt.Sprintf("chat: message action %q is already registered", a.ID))
	}
	x.actions[a.ID] = a
}

// Command returns the command called name.
func (x *Extensions) Command(name string) (app.Command, bool) {
	x.mu.RLock()
	defer x.mu.RUnlock()
	c, ok := x.commands[name]
	return c, ok
}

// Commands returns every command ordered by name.
func (x *Extensions) Commands() []app.Command {
	x.mu.RLock()
	defer x.mu.RUnlock()
	out := make([]app.Command, 0, len(x.commands))
	for _, c := range x.commands {
		out = append(out, c)
	}
	slices.SortFunc(out, func(a, b app.Command) int { return strings.Compare(a.Name, b.Name) })
	return out
}

// MessageActions returns every message action ordered by ID.
func (x *Extensions) MessageActions() []app.MessageAction {
	x.mu.RLock()
	defer x.mu.RUnlock()
	out := make([]app.MessageAction, 0, len(x.actions))
	for _, a := range x.actions {
		out = append(out, a)
	}
	slices.SortFunc(out, func(a, b app.MessageAction) int { return strings.Compare(a.ID, b.ID) })
	return out
}

// CommandRequest is what a client sends to run a command.
type CommandRequest struct {
	Name         string
	Args         string
	ThreadRootID string
}

// RunCommand runs a command for p in a channel p can read. An unreadable
// channel and an unknown command are both not found; the channel is checked
// first so a private channel's existence does not leak.
func (s *Service) RunCommand(ctx context.Context, p app.Principal, channelID string, req CommandRequest) (app.CommandResult, error) {
	v, err := s.channel(ctx, p, channelID)
	if err != nil {
		return app.CommandResult{}, err
	}
	if s.ext == nil {
		return app.CommandResult{}, fmt.Errorf("command %q: %w", req.Name, app.ErrNotFound)
	}
	cmd, ok := s.ext.Command(req.Name)
	if !ok {
		return app.CommandResult{}, fmt.Errorf("command %q: %w", req.Name, app.ErrNotFound)
	}
	args := strings.TrimSpace(req.Args)
	if utf8.RuneCountInString(args) > domain.MaxBodyRunes || strings.ContainsRune(args, 0) {
		return app.CommandResult{}, fmt.Errorf("args must be at most %d characters: %w", domain.MaxBodyRunes, app.ErrInvalid)
	}
	if req.ThreadRootID != "" {
		if _, err := uuid.Parse(req.ThreadRootID); err != nil {
			return app.CommandResult{}, fmt.Errorf("thread_root_id is not a valid ID: %w", app.ErrInvalid)
		}
		if _, err := s.threadRoot(ctx, p, v.ID, req.ThreadRootID); err != nil {
			return app.CommandResult{}, err
		}
	}
	return cmd.Handler(ctx, app.CommandInput{ChannelID: v.ID, ThreadRootID: req.ThreadRootID, Args: args})
}
