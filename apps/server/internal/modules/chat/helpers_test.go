package chat_test

import "github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace"

func workspaceMemberRemoved(ws, userID, actorID string) workspace.MemberRemoved {
	return workspace.MemberRemoved{WorkspaceID: ws, UserID: userID, ActorID: actorID}
}
