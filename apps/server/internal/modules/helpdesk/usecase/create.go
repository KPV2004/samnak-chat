package usecase

import (
	"context"
	"fmt"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/domain"
)

// CreateInput is what a requester chooses. The workspace and the requester come
// from the Principal.
type CreateInput struct {
	CategoryID string
	Title      string
	// Description is optional. When it is empty and SourceMessageID is set, the
	// message's body becomes the description (HD-03).
	Description     string
	Priority        domain.Priority
	SourceMessageID string
	// SourceChannelID is the channel a ticket made by the "/ticket" command
	// comes from; the requester's card goes there. A source message sets it
	// by itself. Only the command sets it: the form never does.
	SourceChannelID string
	// AttachmentIDs are files from RequestUpload that the requester uploaded.
	AttachmentIDs []string
}

// Create stores a ticket in the queue of its category, numbers it within that
// queue (HD-04) and records the creation (HD-18), in one transaction with the
// ticket.created event (HD-20). The cards follow from the event, after the
// commit. Any active member may create a ticket.
func (s *Service) Create(ctx context.Context, p app.Principal, in CreateInput) (domain.Ticket, error) {
	if !validID(in.CategoryID) {
		return domain.Ticket{}, invalid("category_id is not a valid ID")
	}
	fileIDs, err := parseAttachmentIDs(in.AttachmentIDs)
	if err != nil {
		return domain.Ticket{}, err
	}
	cat, err := s.d.Repo.Category(ctx, p.WorkspaceID, in.CategoryID)
	if err != nil {
		if isNotFound(err) {
			return domain.Ticket{}, invalid("category_id is not a category of this workspace")
		}
		return domain.Ticket{}, err
	}
	if cat.ArchivedAt != nil {
		return domain.Ticket{}, invalid("category is archived")
	}
	if err := s.checkQueueOpen(ctx, p.WorkspaceID, cat.QueueID); err != nil {
		return domain.Ticket{}, err
	}

	var fromMessage []app.MessageAttachment
	in2 := domain.NewTicketInput{
		WorkspaceID: p.WorkspaceID, QueueID: cat.QueueID, CategoryID: cat.ID, RequesterID: p.UserID,
		Title: in.Title, Description: in.Description, Priority: in.Priority, SourceChannelID: in.SourceChannelID,
	}
	if in.SourceMessageID != "" {
		msg, err := s.readSource(ctx, p, in.SourceMessageID)
		if err != nil {
			return domain.Ticket{}, err
		}
		in2.SourceChannelID, in2.SourceMessageID = msg.ChannelID, msg.ID
		if in2.Description == "" {
			in2.Description = msg.Body
		}
		fromMessage = msg.Attachments
	}
	if err := s.checkUploads(ctx, p, fileIDs); err != nil {
		return domain.Ticket{}, err
	}
	t, err := domain.NewTicket(in2, newID(), s.clock())
	if err != nil {
		return domain.Ticket{}, err
	}

	err = s.d.Tx.Do(ctx, func(ctx context.Context) error {
		prefix, number, err := s.d.Repo.NextTicketNumber(ctx, p.WorkspaceID, t.QueueID)
		if err != nil {
			return err
		}
		t.Number, t.Key = number, domain.TicketKey(prefix, number)
		if err := s.d.Repo.InsertTicket(ctx, t); err != nil {
			return err
		}
		hist := domain.HistoryEntry{ID: newID(), TicketID: t.ID, ActorID: p.UserID, Action: domain.ActionCreated, At: t.CreatedAt}
		if err := s.d.Repo.InsertHistory(ctx, hist); err != nil {
			return err
		}
		if err := s.attach(ctx, p, t, fileIDs, fromMessage); err != nil {
			return err
		}
		return s.publish(ctx, domain.TicketCreated{TicketRef: ref(t)})
	})
	if err != nil {
		return domain.Ticket{}, err
	}
	return t, nil
}

// attach gives the ticket its uploaded files, which must all still be unused,
// and copies the files of the source message with chat's keys (HD-03).
func (s *Service) attach(ctx context.Context, p app.Principal, t domain.Ticket, fileIDs []string, fromMessage []app.MessageAttachment) error {
	if len(fileIDs) > 0 {
		n, err := s.d.Repo.LinkAttachments(ctx, p.WorkspaceID, t.ID, p.UserID, fileIDs)
		if err != nil {
			return err
		}
		if n != int64(len(fileIDs)) {
			// Another ticket took a file between the check and now.
			return fmt.Errorf("an attachment is already used by a ticket: %w", app.ErrConflict)
		}
	}
	for _, m := range fromMessage {
		a := domain.Attachment{
			ID: newID(), TicketID: t.ID, WorkspaceID: t.WorkspaceID, UploaderID: p.UserID, ObjectKey: m.ObjectKey,
			FileName: m.FileName, Size: m.Size, MimeType: m.MimeType, FromMessage: true, CreatedAt: t.CreatedAt,
		}
		if err := s.d.Repo.InsertAttachment(ctx, a); err != nil {
			return err
		}
	}
	return nil
}

// checkQueueOpen refuses a queue that is archived or gone, as a 400: the
// category was named in the body.
func (s *Service) checkQueueOpen(ctx context.Context, workspaceID, queueID string) error {
	q, err := s.d.Repo.Queue(ctx, workspaceID, queueID)
	if err != nil {
		return err
	}
	if q.ArchivedAt != nil {
		return invalid("queue %s is archived", q.Prefix)
	}
	return nil
}

// readSource reads the chat message the ticket is made from, as the requester.
// Without chat the message cannot be read, so the request is refused (400);
// a message the requester cannot read is not found (404).
func (s *Service) readSource(ctx context.Context, p app.Principal, messageID string) (app.MessageSnapshot, error) {
	if !validID(messageID) {
		return app.MessageSnapshot{}, invalid("source_message_id is not a valid ID")
	}
	if s.d.Messages == nil {
		return app.MessageSnapshot{}, invalid("source_message_id needs chat, which is not available")
	}
	msg, err := s.d.Messages.ReadMessage(ctx, p.WorkspaceID, messageID, p.UserID)
	if err != nil {
		return app.MessageSnapshot{}, fmt.Errorf("source message: %w", err)
	}
	return msg, nil
}

// ref is what every ticket event carries.
func ref(t domain.Ticket) domain.TicketRef {
	return domain.TicketRef{
		WorkspaceID: t.WorkspaceID, TicketID: t.ID, QueueID: t.QueueID, Key: t.Key, RequesterID: t.RequesterID,
	}
}
