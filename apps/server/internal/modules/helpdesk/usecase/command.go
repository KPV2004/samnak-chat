package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

// CommandName is the slash command that opens a ticket from chat (HD-03).
const CommandName = "ticket"

// ActionCreateFromMessage is the message action the web shows as "Create
// ticket": it opens the new-ticket form with the message text (HD-03).
const ActionCreateFromMessage = "helpdesk.create_from_message"

// CreateFromCommand runs "/ticket <title>" for the caller in a channel: the
// ticket goes to the default category and its requester card to that channel.
// Without a default category the command is refused with 400 and points at the
// form, which always names a category (HD-03).
func (s *Service) CreateFromCommand(ctx context.Context, p app.Principal, in app.CommandInput) error {
	title := strings.TrimSpace(in.Args)
	if title == "" {
		return invalid("usage: /ticket <title>")
	}
	cat, err := s.d.Repo.DefaultCategory(ctx, p.WorkspaceID)
	if err != nil {
		if isNotFound(err) {
			return invalid("no_default_category: open the ticket form to choose a category")
		}
		return fmt.Errorf("helpdesk: default category: %w", err)
	}
	_, err = s.Create(ctx, p, CreateInput{CategoryID: cat.ID, Title: title, SourceChannelID: in.ChannelID})
	return err
}
