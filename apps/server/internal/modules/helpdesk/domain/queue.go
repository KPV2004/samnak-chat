package domain

import "time"

// Queue is where tickets of one kind go, such as IT or HR (HD-01). Its agents
// are the people who take its tickets. An empty ChannelID means no agents'
// channel.
type Queue struct {
	ID          string
	WorkspaceID string
	Name        string
	Prefix      string
	ChannelID   string
	AgentIDs    []string
	ArchivedAt  *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// QueueInput is what an admin chooses when creating a queue.
type QueueInput struct {
	Name      string
	Prefix    string
	ChannelID string
}

// NewQueue builds a queue with a checked name and prefix. The caller supplies
// the ID and the time so it controls identity and the clock.
func NewQueue(in QueueInput, id, workspaceID string, now time.Time) (Queue, error) {
	name, err := validName(in.Name)
	if err != nil {
		return Queue{}, err
	}
	prefix, err := ValidPrefix(in.Prefix)
	if err != nil {
		return Queue{}, err
	}
	now = now.UTC()
	return Queue{
		ID: id, WorkspaceID: workspaceID, Name: name, Prefix: prefix, ChannelID: in.ChannelID,
		CreatedAt: now, UpdatedAt: now,
	}, nil
}

// QueuePatch is a partial edit. A nil pointer leaves a field alone. The
// channel can be cleared, so it carries an explicit flag: ChannelSet with an
// empty ChannelID clears it.
type QueuePatch struct {
	Name       *string
	ChannelSet bool
	ChannelID  string
	Archived   *bool
}

// Apply changes q as p says. A rejected name leaves q as it was.
func (q *Queue) Apply(p QueuePatch, now time.Time) error {
	name := q.Name
	if p.Name != nil {
		var err error
		if name, err = validName(*p.Name); err != nil {
			return err
		}
	}
	now = now.UTC()
	q.Name = name
	if p.ChannelSet {
		q.ChannelID = p.ChannelID
	}
	if p.Archived != nil {
		q.ArchivedAt = nil
		if *p.Archived {
			q.ArchivedAt = &now
		}
	}
	q.UpdatedAt = now
	return nil
}

// IsAgent reports whether userID is an agent of the queue.
func (q Queue) IsAgent(userID string) bool {
	if userID == "" {
		return false
	}
	for _, id := range q.AgentIDs {
		if id == userID {
			return true
		}
	}
	return false
}
