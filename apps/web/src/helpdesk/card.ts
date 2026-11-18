import type { TicketPriority, TicketStatus } from "@samnak/core";

/**
 * The payload of a `helpdesk.ticket` card (HD-08). `can_claim_queue_id` names the queue whose
 * agents may claim the ticket while it is unassigned; the viewer must still be one of them.
 * `version` is the newest-wins number of the card, as the approval and task cards have.
 */
export interface HelpdeskCardPayload {
  ticket_id: string;
  key: string;
  title: string;
  status: TicketStatus;
  priority: TicketPriority;
  queue_name: string;
  requester_id: string;
  requester_name: string;
  assignee_id: string | null;
  assignee_name: string;
  can_claim_queue_id: string | null;
  version: number;
}

const statuses: readonly string[] = ["new", "open", "in_progress", "waiting", "resolved", "closed"] satisfies TicketStatus[];
const priorities: readonly string[] = ["low", "normal", "high", "urgent"] satisfies TicketPriority[];

const text = (value: unknown) => (typeof value === "string" ? value : "");
const nullableText = (value: unknown) => (typeof value === "string" && value !== "" ? value : null);

/** Reads a card payload; null when it is not a ticket card this client understands. */
export function parseHelpdeskCard(payload: unknown): HelpdeskCardPayload | null {
  if (typeof payload !== "object" || payload === null) return null;
  const p = payload as Record<string, unknown>;
  if (typeof p.ticket_id !== "string" || p.ticket_id === "") return null;
  if (typeof p.status !== "string" || !statuses.includes(p.status)) return null;
  if (typeof p.priority !== "string" || !priorities.includes(p.priority)) return null;
  return {
    ticket_id: p.ticket_id,
    key: text(p.key),
    title: text(p.title),
    status: p.status as TicketStatus,
    priority: p.priority as TicketPriority,
    queue_name: text(p.queue_name),
    requester_id: text(p.requester_id),
    requester_name: text(p.requester_name),
    assignee_id: nullableText(p.assignee_id),
    assignee_name: text(p.assignee_name),
    can_claim_queue_id: nullableText(p.can_claim_queue_id),
    version: typeof p.version === "number" ? p.version : 0,
  };
}
