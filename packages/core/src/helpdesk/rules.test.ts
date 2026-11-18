import { describe, expect, it } from "vitest";
import type { Ticket } from "../api/helpdesk";
import { canAssign, canClaim, canEditTicket, canWriteInternal, statusMoves, type Viewer } from "./rules";

const day = 24 * 60 * 60 * 1000;

function ticket(patch: Partial<Ticket> = {}): Ticket {
  return {
    id: "t1",
    key: "IT-0142",
    number: 142,
    queue_id: "q1",
    category_id: "c1",
    title: "Printer on fire",
    description: "",
    requester_id: "u1",
    assignee_id: null,
    status: "new",
    priority: "normal",
    source_channel_id: null,
    source_message_id: null,
    message_id: null,
    resolved_at: null,
    closed_at: null,
    created_at: "2026-01-01T09:00:00Z",
    updated_at: "2026-01-01T09:00:00Z",
    ...patch,
  };
}

const requester: Viewer = { userId: "u1", admin: false, agentOfQueue: false };
const agent: Viewer = { userId: "u2", admin: false, agentOfQueue: true };
const otherAgent: Viewer = { userId: "u3", admin: false, agentOfQueue: true };
const admin: Viewer = { userId: "u9", admin: true, agentOfQueue: false };
const now = new Date("2026-01-10T09:00:00Z");

describe("claim and assign", () => {
  it("lets an agent of the queue claim an unassigned ticket", () => {
    expect(canClaim(ticket(), agent)).toBe(true);
  });

  it("does not offer a claim to a requester, a stranger or an admin who is not an agent", () => {
    expect(canClaim(ticket(), requester)).toBe(false);
    expect(canClaim(ticket(), admin)).toBe(false);
  });

  it("offers no claim once the ticket has an assignee or is closed", () => {
    expect(canClaim(ticket({ assignee_id: "u3", status: "open" }), agent)).toBe(false);
    expect(canClaim(ticket({ status: "closed", closed_at: "2026-01-02T00:00:00Z" }), agent)).toBe(false);
  });

  it("lets an admin assign a ticket in any state except closed", () => {
    expect(canAssign(ticket({ assignee_id: "u2", status: "in_progress" }), admin)).toBe(true);
    expect(canAssign(ticket({ status: "closed", closed_at: "2026-01-02T00:00:00Z" }), admin)).toBe(false);
  });

  it("never lets an agent assign, only claim", () => {
    expect(canAssign(ticket(), agent)).toBe(false);
  });
});

describe("status moves (HD-05)", () => {
  it("gives the assignee and an admin the handling moves", () => {
    const handled = ticket({ assignee_id: "u2", status: "open" });
    expect(statusMoves(handled, agent, now)).toEqual(["in_progress"]);
    expect(statusMoves(handled, admin, now)).toEqual(["in_progress"]);
  });

  it("gives the other agents of the queue nothing on someone else's ticket", () => {
    expect(statusMoves(ticket({ assignee_id: "u2", status: "in_progress" }), otherAgent, now)).toEqual([]);
  });

  it("walks the handler's path: waiting, back, and resolved", () => {
    expect(statusMoves(ticket({ assignee_id: "u2", status: "in_progress" }), agent, now)).toEqual(["waiting", "resolved"]);
    expect(statusMoves(ticket({ assignee_id: "u2", status: "waiting" }), agent, now)).toEqual(["in_progress"]);
  });

  it("lets the requester or an admin close a resolved ticket, and the requester reopen it within 7 days", () => {
    const resolved = ticket({ assignee_id: "u2", status: "resolved", resolved_at: "2026-01-08T09:00:00Z" });
    expect(statusMoves(resolved, requester, now)).toEqual(["closed", "open"]);
    expect(statusMoves(resolved, admin, now)).toEqual(["closed"]);
    expect(statusMoves(resolved, agent, now)).toEqual([]);
  });

  it("refuses the reopen after 7 days, but keeps the close", () => {
    const resolvedAt = new Date(now.getTime() - 7 * day - 1000).toISOString();
    const resolved = ticket({ assignee_id: "u2", status: "resolved", resolved_at: resolvedAt });
    expect(statusMoves(resolved, requester, now)).toEqual(["closed"]);
  });

  it("gives nobody a move on a closed ticket, and no status move on a new one", () => {
    expect(statusMoves(ticket({ status: "closed", closed_at: "2026-01-02T00:00:00Z" }), requester, now)).toEqual([]);
    expect(statusMoves(ticket(), admin, now)).toEqual([]);
  });
});

describe("internal notes and edits", () => {
  it("lets only agents and admins write internal notes and change priority or category", () => {
    expect(canWriteInternal(agent)).toBe(true);
    expect(canWriteInternal(admin)).toBe(true);
    expect(canWriteInternal(requester)).toBe(false);
    expect(canEditTicket(agent)).toBe(true);
    expect(canEditTicket(requester)).toBe(false);
  });
});
