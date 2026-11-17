// Package approval owns approval requests: leave and general requests, the
// approver's decision, the history and the chat card (SRS section 12). This
// file and events.go are the only parts other modules may import.
//
// The module posts an `approval.request` card in the DM between requester and
// approver and registers notification rules for its events. Other modules
// listen for the events in events.go; there is nothing to call.
package approval
