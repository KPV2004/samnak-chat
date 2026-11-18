// Package wire holds the JSON shapes of the helpdesk REST API. Type names
// start with "Helpdesk" because Huma names schemas after their Go types and
// those names are shared by every module's operations.
package wire

import (
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/usecase"
)

// HelpdeskCategory is a category of a queue (HD-02).
type HelpdeskCategory struct {
	ID        string `json:"id"`
	QueueID   string `json:"queue_id"`
	Name      string `json:"name"`
	IsDefault bool   `json:"is_default" doc:"The category new tickets from chat take; one per workspace"`
}

// HelpdeskQueue is a queue with its agents (HD-01). Categories are listed by
// GET /helpdesk/queues; the other queue responses leave them empty.
type HelpdeskQueue struct {
	ID         string             `json:"id"`
	Name       string             `json:"name"`
	Prefix     string             `json:"prefix" doc:"The ticket key prefix, e.g. IT for IT-0142"`
	ChannelID  *string            `json:"channel_id" doc:"The agents' channel; null when the queue has none"`
	AgentIDs   []string           `json:"agent_ids" nullable:"false"`
	Categories []HelpdeskCategory `json:"categories" nullable:"false"`
}

// ToQueue converts a stored queue with its active categories.
func ToQueue(q domain.Queue, cats []domain.Category) HelpdeskQueue {
	out := HelpdeskQueue{
		ID: q.ID, Name: q.Name, Prefix: q.Prefix, AgentIDs: append([]string{}, q.AgentIDs...),
		Categories: make([]HelpdeskCategory, 0, len(cats)),
	}
	if q.ChannelID != "" {
		id := q.ChannelID
		out.ChannelID = &id
	}
	for _, c := range cats {
		out.Categories = append(out.Categories, ToCategory(c))
	}
	return out
}

// ToQueues converts the queue summaries of GET /helpdesk/queues.
func ToQueues(list []usecase.QueueSummary) []HelpdeskQueue {
	out := make([]HelpdeskQueue, len(list))
	for i, s := range list {
		out[i] = ToQueue(s.Queue, s.Categories)
	}
	return out
}

// ToCategory converts a stored category.
func ToCategory(c domain.Category) HelpdeskCategory {
	return HelpdeskCategory{ID: c.ID, QueueID: c.QueueID, Name: c.Name, IsDefault: c.IsDefault}
}
