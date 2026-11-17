package app

import "context"

// ChatExtensions lets modules add slash commands and message actions to chat
// (CHAT-16). chat provides it in Provide; look it up in Init and skip
// registration when it is missing. Only enabled modules register, so a
// disabled module's commands and actions disappear with it.
type ChatExtensions interface {
	RegisterCommand(c Command)
	RegisterMessageAction(a MessageAction)
}

// Command is a slash command such as "/task Fix the login page".
type Command struct {
	Name        string // without the slash, lower case, unique, e.g. "task"
	Module      string
	Usage       string // e.g. "/task <title> [@assignee]"
	Description string // English; clients may localise known names
	Handler     CommandHandler
}

// CommandInput is what the user typed and where. The Principal is in ctx.
type CommandInput struct {
	ChannelID    string
	ThreadRootID string // set when typed in a thread
	Args         string // the text after "/name ", trimmed
}

// CommandResult is shown only to the caller. A command that creates
// something posts its own card instead of returning text.
type CommandResult struct {
	Text string
}

// CommandHandler runs a command. Return an error wrapping ErrInvalid for bad
// arguments (400) or ErrForbidden (403); other errors are 500.
type CommandHandler func(ctx context.Context, in CommandInput) (CommandResult, error)

// MessageAction is an item in a message's menu, e.g. "Create task". The
// client opens its own form for a known ID and hides unknown ones.
type MessageAction struct {
	ID     string // "<module>.<name>", e.g. "task.create_from_message"
	Module string
}

// ChannelAccess answers whether a user may read a channel, with chat's rules
// (public channels for any active member, private channels and DMs for their
// members). chat provides it. Modules use it to show channel-bound objects,
// such as tasks, only to people who can see the channel.
type ChannelAccess interface {
	CanRead(ctx context.Context, workspaceID, channelID, userID string) (bool, error)
}
