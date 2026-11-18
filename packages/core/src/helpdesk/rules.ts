import type { Ticket, TicketStatus } from "../api/helpdesk";

/**
 * Who is looking at a ticket. `admin` is the workspace owner or admin; `agentOfQueue` is
 * whether the viewer is listed as an agent of the ticket's queue.
 */
export interface Viewer {
  userId: string;
  admin: boolean;
  agentOfQueue: boolean;
}

/** How long after resolution the requester may reopen a ticket (HD-05). */
export const reopenWindowMs = 7 * 24 * 60 * 60 * 1000;

type Who = "handler" | "requesterOrAdmin" | "requester";

/**
 * The status moves of HD-05 that go through the status endpoint. The server enforces the same
 * table; the web uses it to show only the buttons the viewer may press.
 */
const moves: { from: TicketStatus; to: TicketStatus; who: Who; reopen?: boolean }[] = [
  { from: "open", to: "in_progress", who: "handler" },
  { from: "in_progress", to: "waiting", who: "handler" },
  { from: "waiting", to: "in_progress", who: "handler" },
  { from: "in_progress", to: "resolved", who: "handler" },
  { from: "resolved", to: "closed", who: "requesterOrAdmin" },
  { from: "resolved", to: "open", who: "requester", reopen: true },
];

function isHandler(ticket: Ticket, viewer: Viewer): boolean {
  return ticket.assignee_id === viewer.userId || viewer.admin;
}

/**
 * An agent of the queue may take an unassigned ticket (HD-06). Claiming a ticket that is already
 * theirs does nothing, so it is not offered either.
 */
export function canClaim(ticket: Ticket, viewer: Viewer): boolean {
  return viewer.agentOfQueue && ticket.assignee_id === null && ticket.status !== "closed";
}

/** Admins assign any agent of the queue, in any state except closed (HD-06). */
export function canAssign(ticket: Ticket, viewer: Viewer): boolean {
  return viewer.admin && ticket.status !== "closed";
}

/** The statuses the viewer may move the ticket to now, in the order of the state table. */
export function statusMoves(ticket: Ticket, viewer: Viewer, now: Date): TicketStatus[] {
  return moves
    .filter((m) => m.from === ticket.status)
    .filter((m) => {
      if (m.who === "handler") return isHandler(ticket, viewer);
      if (m.who === "requesterOrAdmin") return viewer.userId === ticket.requester_id || viewer.admin;
      return viewer.userId === ticket.requester_id;
    })
    .filter((m) => !m.reopen || reopenAllowed(ticket, now))
    .map((m) => m.to);
}

function reopenAllowed(ticket: Ticket, now: Date): boolean {
  return ticket.resolved_at !== null && now.getTime() <= Date.parse(ticket.resolved_at) + reopenWindowMs;
}

/** Internal comments and their history lines are for agents and admins (HD-07). */
export function canWriteInternal(viewer: Viewer): boolean {
  return viewer.admin || viewer.agentOfQueue;
}

/** Priority and category are changed by agents and admins only (HD-09). */
export function canEditTicket(viewer: Viewer): boolean {
  return viewer.admin || viewer.agentOfQueue;
}
