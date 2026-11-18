package domain

import "time"

// Category is a kind of problem inside a queue (HD-02). A ticket takes the
// queue of its category. Each workspace has at most one default category,
// which new tickets from chat use (HD-03).
type Category struct {
	ID          string
	WorkspaceID string
	QueueID     string
	Name        string
	IsDefault   bool
	ArchivedAt  *time.Time
	CreatedAt   time.Time
}

// CategoryInput is what an admin chooses when creating a category.
type CategoryInput struct {
	QueueID   string
	Name      string
	IsDefault bool
}

// NewCategory builds a category with a checked name.
func NewCategory(in CategoryInput, id, workspaceID string, now time.Time) (Category, error) {
	name, err := validName(in.Name)
	if err != nil {
		return Category{}, err
	}
	return Category{
		ID: id, WorkspaceID: workspaceID, QueueID: in.QueueID, Name: name, IsDefault: in.IsDefault,
		CreatedAt: now.UTC(),
	}, nil
}

// CategoryPatch is a partial edit. A nil pointer leaves a field alone.
type CategoryPatch struct {
	Name      *string
	QueueID   *string
	IsDefault *bool
	Archived  *bool
}

// Apply changes c as p says. An archived category is never the default.
func (c *Category) Apply(p CategoryPatch, now time.Time) error {
	name := c.Name
	if p.Name != nil {
		var err error
		if name, err = validName(*p.Name); err != nil {
			return err
		}
	}
	queueID := c.QueueID
	if p.QueueID != nil {
		if *p.QueueID == "" {
			return invalid("queue_id is required")
		}
		queueID = *p.QueueID
	}
	c.Name, c.QueueID = name, queueID
	if p.IsDefault != nil {
		c.IsDefault = *p.IsDefault
	}
	if p.Archived != nil {
		c.ArchivedAt = nil
		if *p.Archived {
			at := now.UTC()
			c.ArchivedAt = &at
			c.IsDefault = false
		}
	}
	return nil
}
