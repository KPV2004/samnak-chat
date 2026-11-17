package domain

// The events of the approval module (APR-13). They are published inside the
// transaction that changes the request, so they are delivered after the
// commit and at least once (PLT-10). Every one carries the whole picture a
// listener needs, so a rule or a card handler never has to guess who is who.
// Fields may be added, never removed.

// RequestCreated is published when a request is submitted.
type RequestCreated struct {
	WorkspaceID string `json:"workspace_id"`
	RequestID   string `json:"request_id"`
	Type        string `json:"type"`
	RequesterID string `json:"requester_id"`
	ApproverID  string `json:"approver_id"`
	ActorID     string `json:"actor_id"`
}

func (RequestCreated) EventName() string { return "approval.request_created" }

// RequestApproved is published when the approver approves.
type RequestApproved struct {
	WorkspaceID string `json:"workspace_id"`
	RequestID   string `json:"request_id"`
	Type        string `json:"type"`
	RequesterID string `json:"requester_id"`
	ApproverID  string `json:"approver_id"`
	ActorID     string `json:"actor_id"`
}

func (RequestApproved) EventName() string { return "approval.request_approved" }

// RequestRejected is published when the approver rejects, with the reason.
type RequestRejected struct {
	WorkspaceID string `json:"workspace_id"`
	RequestID   string `json:"request_id"`
	Type        string `json:"type"`
	RequesterID string `json:"requester_id"`
	ApproverID  string `json:"approver_id"`
	ActorID     string `json:"actor_id"`
	Reason      string `json:"reason"`
}

func (RequestRejected) EventName() string { return "approval.request_rejected" }

// RequestCancelled is published when the requester cancels.
type RequestCancelled struct {
	WorkspaceID string `json:"workspace_id"`
	RequestID   string `json:"request_id"`
	Type        string `json:"type"`
	RequesterID string `json:"requester_id"`
	ApproverID  string `json:"approver_id"`
	ActorID     string `json:"actor_id"`
}

func (RequestCancelled) EventName() string { return "approval.request_cancelled" }
