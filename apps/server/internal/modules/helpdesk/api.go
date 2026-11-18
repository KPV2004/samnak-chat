// Package helpdesk owns queues, categories and tickets: work requests from
// people that agents take and resolve, shown as cards in the chat (SRS
// HD-01..10, 18..20). This file and events.go are the only parts other modules
// may import.
//
// Other modules never call helpdesk. They listen for the events in events.go.
package helpdesk

import "github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/usecase"

// ActionAdmin is the workspace action that manages queues, categories and
// agents and sees every ticket (HD-01). Owners and admins hold it.
const ActionAdmin = usecase.ActionAdmin
