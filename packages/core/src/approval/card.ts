import type { ApprovalStatus, ApprovalType } from "../api/m4-approval";

/**
 * The payload of an `approval.request` card, as the approval module writes it.
 * `version` is the request's `updated_at` in Unix milliseconds: the larger one
 * is the newer card.
 */
export interface ApprovalCardPayload {
  request_id: string;
  type: ApprovalType;
  status: ApprovalStatus;
  /** The leave range or the subject. */
  summary: string;
  /** The request's own payload. */
  fields: Record<string, unknown>;
  requester_id: string;
  requester_name: string;
  approver_id: string;
  approver_name: string;
  /** Why it was rejected; empty otherwise. */
  reason: string;
  decided_at: string | null;
  version: number;
}

const statuses: readonly string[] = ["pending", "approved", "rejected", "cancelled"];
const types: readonly string[] = ["leave", "general"];

const text = (value: unknown) => (typeof value === "string" ? value : "");

/** Reads a card payload; null when it is not an approval card this client understands. */
export function parseApprovalCard(payload: unknown): ApprovalCardPayload | null {
  if (typeof payload !== "object" || payload === null) return null;
  const p = payload as Record<string, unknown>;
  if (typeof p.request_id !== "string" || p.request_id === "") return null;
  if (typeof p.type !== "string" || !types.includes(p.type)) return null;
  if (typeof p.status !== "string" || !statuses.includes(p.status)) return null;
  const fields = typeof p.fields === "object" && p.fields !== null ? (p.fields as Record<string, unknown>) : {};
  return {
    request_id: p.request_id,
    type: p.type as ApprovalType,
    status: p.status as ApprovalStatus,
    summary: text(p.summary),
    fields,
    requester_id: text(p.requester_id),
    requester_name: text(p.requester_name),
    approver_id: text(p.approver_id),
    approver_name: text(p.approver_name),
    reason: text(p.reason),
    decided_at: typeof p.decided_at === "string" ? p.decided_at : null,
    version: typeof p.version === "number" ? p.version : 0,
  };
}

/** The newer of two card payloads by `version`; on a tie the later argument wins. */
export function newestCard(current: ApprovalCardPayload | null, incoming: ApprovalCardPayload | null): ApprovalCardPayload | null {
  if (!current) return incoming;
  if (!incoming) return current;
  return incoming.version >= current.version ? incoming : current;
}

/** The card version a request has: its `updated_at` in Unix milliseconds. */
export function requestVersion(updatedAt: string): number {
  const ms = Date.parse(updatedAt);
  return Number.isNaN(ms) ? 0 : ms;
}
