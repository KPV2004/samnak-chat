import type { ApprovalHistoryAction, ApprovalStatus, ApprovalType } from "@samnak/core";
import type { MessageKey } from "../i18n/messages";
import type { T } from "../i18n/useT";

export const typeKeys = {
  leave: "approval.type.leave",
  general: "approval.type.general",
} as const satisfies Record<ApprovalType, MessageKey>;

export const statusKeys = {
  pending: "approval.status.pending",
  approved: "approval.status.approved",
  rejected: "approval.status.rejected",
  cancelled: "approval.status.cancelled",
} as const satisfies Record<ApprovalStatus, MessageKey>;

export const actionKeys = {
  created: "approval.action.created",
  approved: "approval.action.approved",
  rejected: "approval.action.rejected",
  cancelled: "approval.action.cancelled",
} as const satisfies Record<ApprovalHistoryAction, MessageKey>;

export const leaveTypeKeys = {
  annual: "approval.leaveType.annual",
  sick: "approval.leaveType.sick",
  personal: "approval.leaveType.personal",
  other: "approval.leaveType.other",
} as const satisfies Record<string, MessageKey>;

export const halfDayKeys = {
  morning: "approval.halfDay.morning",
  afternoon: "approval.halfDay.afternoon",
} as const satisfies Record<string, MessageKey>;

const text = (value: unknown) => (typeof value === "string" ? value : "");

/** One line for a request: its leave range, or its subject for a general request. */
export function summarize(type: ApprovalType, payload: Record<string, unknown>, t: T): string {
  if (type === "general") return text(payload.subject);
  const leave = text(payload.leave_type);
  const leaveKey = leaveTypeKeys[leave as keyof typeof leaveTypeKeys];
  return `${leaveKey ? t(leaveKey) : leave} · ${text(payload.start_date)} – ${text(payload.end_date)}`;
}
