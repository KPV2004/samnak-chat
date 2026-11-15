package workspace

// MemberRemoved is published after a member was removed from a workspace
// (WSP-04). Consumers drop that user's access to the workspace's data.
type MemberRemoved struct {
	WorkspaceID string `json:"workspace_id"`
	UserID      string `json:"user_id"`
	ActorID     string `json:"actor_id"`
}

func (MemberRemoved) EventName() string { return "workspace.member_removed" }

// MemberDeactivated is published after a member was deactivated (WSP-08).
// Consumers close that user's connections to the workspace. Reactivating a
// member publishes nothing.
type MemberDeactivated struct {
	WorkspaceID string `json:"workspace_id"`
	UserID      string `json:"user_id"`
	ActorID     string `json:"actor_id"`
}

func (MemberDeactivated) EventName() string { return "workspace.member_deactivated" }
