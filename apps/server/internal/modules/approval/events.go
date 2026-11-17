package approval

import "github.com/KPV2004/samnak-chat/apps/server/internal/modules/approval/domain"

// Events other modules may listen for (APR-13). Each is published inside the
// transaction that changed the request, so it is delivered once the change
// committed, at least once.
type (
	// RequestCreated is `approval.request_created`.
	RequestCreated = domain.RequestCreated
	// RequestApproved is `approval.request_approved`.
	RequestApproved = domain.RequestApproved
	// RequestRejected is `approval.request_rejected`; it carries the reason.
	RequestRejected = domain.RequestRejected
	// RequestCancelled is `approval.request_cancelled`.
	RequestCancelled = domain.RequestCancelled
)
