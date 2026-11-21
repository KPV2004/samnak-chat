package httpadapter

import (
	"context"

	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/chat/usecase"
)

type commandInfo struct {
	Name        string `json:"name" doc:"Without the slash"`
	Usage       string `json:"usage" doc:"For example /task <title> [@assignee]"`
	Description string `json:"description"`
	Module      string `json:"module"`
}

type messageActionInfo struct {
	ID     string `json:"id" doc:"<module>.<name>; a client hides IDs it does not know"`
	Module string `json:"module"`
}

type extensionsOutput struct {
	Body struct {
		Commands []commandInfo       `json:"commands" nullable:"false"`
		Actions  []messageActionInfo `json:"message_actions" nullable:"false"`
	}
}

func (h *Handler) listExtensions(ctx context.Context, _ *struct{}) (*extensionsOutput, error) {
	if _, err := principal(ctx); err != nil {
		return nil, h.fail(ctx, err)
	}
	out := &extensionsOutput{}
	cmds, actions := h.ext.Commands(), h.ext.MessageActions()
	out.Body.Commands = make([]commandInfo, len(cmds))
	for i, c := range cmds {
		out.Body.Commands[i] = commandInfo{Name: c.Name, Usage: c.Usage, Description: c.Description, Module: c.Module}
	}
	out.Body.Actions = make([]messageActionInfo, len(actions))
	for i, a := range actions {
		out.Body.Actions[i] = messageActionInfo{ID: a.ID, Module: a.Module}
	}
	return out, nil
}

type runCommandInput struct {
	ID   string `path:"id"`
	Body struct {
		Name         string `json:"name" maxLength:"32" doc:"The command, without the slash"`
		Args         string `json:"args,omitempty" maxLength:"4000" doc:"The text after the command name; at most 4,000 characters"`
		ThreadRootID string `json:"thread_root_id,omitempty" maxLength:"36" doc:"Set when the command is typed in a thread"`
	}
}

type runCommandOutput struct {
	Body struct {
		Text string `json:"text" doc:"Shown to the caller only; empty when the command posted something itself"`
	}
}

func (h *Handler) runCommand(ctx context.Context, in *runCommandInput) (*runCommandOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	res, err := h.uc.RunCommand(ctx, p, in.ID, usecase.CommandRequest{Name: in.Body.Name, Args: in.Body.Args, ThreadRootID: in.Body.ThreadRootID})
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	out := &runCommandOutput{}
	out.Body.Text = res.Text
	return out, nil
}
