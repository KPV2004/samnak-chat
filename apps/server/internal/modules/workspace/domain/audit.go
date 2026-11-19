package domain

import "time"

// AuditAction names an admin action in the audit log (WSP-10).
type AuditAction string

// The actions the log records. Each one is written by the use case that
// makes the change, in the same transaction.
const (
	AuditInviteCreated   AuditAction = "invite.created"
	AuditInviteRevoked   AuditAction = "invite.revoked"
	AuditMemberUpdated   AuditAction = "member.updated"
	AuditMemberRemoved   AuditAction = "member.removed"
	AuditGroupCreated    AuditAction = "group.created"
	AuditGroupUpdated    AuditAction = "group.updated"
	AuditGroupDeleted    AuditAction = "group.deleted"
	AuditGroupMembersSet AuditAction = "group.members_set"
)

// AuditTargetType says what kind of row AuditEntry.TargetID refers to.
type AuditTargetType string

// Target types of the audit log.
const (
	TargetMember AuditTargetType = "member"
	TargetInvite AuditTargetType = "invite"
	TargetGroup  AuditTargetType = "group"
)

// AuditEntry is one row of the audit log. Data holds what the action changed,
// for example {"role": {"from": "member", "to": "manager"}}; it is an empty
// object when the action has nothing more to say.
type AuditEntry struct {
	ID          string
	WorkspaceID string
	ActorID     string
	Action      AuditAction
	TargetType  AuditTargetType
	TargetID    string
	Data        map[string]any
	CreatedAt   time.Time
}

// Change is a before and after pair in AuditEntry.Data.
type Change[T any] struct {
	From T `json:"from"`
	To   T `json:"to"`
}
